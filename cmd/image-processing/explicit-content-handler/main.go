package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/akashrv/knative-samples/pkg/eventsschema"

	"cloud.google.com/go/storage"
	cloudevents "github.com/cloudevents/sdk-go"
)

var (
	quarantineBucketID = os.Getenv("QUARANTINE_BUCKET_ID")
	eventType          = "custom.fileuploaded"
)

func receive(ctx context.Context, event cloudevents.Event, response *cloudevents.EventResponse) error {
	if status, err := processEvent(ctx, event); err != nil {
		response.Error(status, err.Error())
		log.Println("Error occured:", err.Error())
	}
	return nil
}

func processEvent(ctx context.Context, event cloudevents.Event) (int, error) {

	if event.Context == nil {
		return http.StatusBadRequest, fmt.Errorf("event.Context is nil. cloudevents.Event\n%s", event.String())
	}

	if event.Context.GetType() != eventType {
		return http.StatusBadRequest, fmt.Errorf("invalid event type %s. Supported event type: %s", event.Context.GetType(), eventType)
	}

	data := eventsschema.FileUploaded{}
	err := event.DataAs(&data)

	if err != nil {
		return http.StatusBadRequest, fmt.Errorf("couldn't convert event.Data to eventsschema.FileUploaded. %s", err.Error())
	}

	if err := moveObject(ctx, data.BucketID, data.ObjectID, quarantineBucketID); err != nil {
		return http.StatusInternalServerError, err
	}

	log.Printf("Successfully quarantined  gs://%s/%s to gs://%s/%s", data.BucketID, data.ObjectID, quarantineBucketID, data.ObjectID)

	return 0, nil
}

func moveObject(ctx context.Context, sourceBucketID, objectID, targetBucketID string) error {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return err
	}

	sbh := client.Bucket(sourceBucketID)
	if _, err := sbh.Attrs(ctx); err != nil {
		return fmt.Errorf("Bucket with BucketID:%s doesn't exist. %s", sourceBucketID, err.Error())
	}
	tbh := client.Bucket(targetBucketID)
	if _, err := sbh.Attrs(ctx); err != nil {
		return fmt.Errorf("Bucket with BucketID:%s doesn't exist. %s", sourceBucketID, err.Error())
	}
	soh := sbh.Object(objectID)
	toh := tbh.Object(objectID)
	copier := toh.CopierFrom(soh)
	if _, err := copier.Run(ctx); err != nil {
		return err
	}

	log.Printf("Object %s copied from bucket:%s to bucket%s", objectID, sourceBucketID, targetBucketID)

	if err := soh.Delete(ctx); err != nil {
		return fmt.Errorf("Failed to delete object:%s in bucket:%s", objectID, sourceBucketID)
	}

	log.Printf("Object %s from bucket %s successfully deleted.", objectID, sourceBucketID)

	return nil
}

func main() {
	c, err := cloudevents.NewDefaultClient()
	if err != nil {
		log.Fatalf("failed to create client, %v", err)
	}
	if quarantineBucketID == "" {
		log.Fatalf("Environment variable QUARANTINE_BUCKET_ID not set.")
	}
	log.Fatal(c.StartReceiver(context.Background(), receive))
}
