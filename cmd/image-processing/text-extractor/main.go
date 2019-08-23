package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"cloud.google.com/go/storage"
	vision "cloud.google.com/go/vision/apiv1"
	"github.com/akashrv/knative-samples/pkg/eventsschema"
	cloudevents "github.com/cloudevents/sdk-go"
)

var (
	eventType = "custom.fileuploaded"
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

	objectURI := fmt.Sprintf("gs://%s/%s", data.BucketID, data.ObjectID)
	// log.Printf("ObjectURI:%s\n", objectURI)

	text, err := extractText(ctx, objectURI)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	log.Printf("Text extracted from object %q: %q", objectURI, text)

	if text != "" {
		if err := writeToGcs(ctx, data.BucketID, data.ObjectID+".txt", text); err != nil {
			return http.StatusInternalServerError, err
		}
		log.Printf("successfully uploaded extracted text to bucket:%s object:%s", data.BucketID, data.ObjectID+".txt")
	}

	return 0, nil
}
func writeToGcs(ctx context.Context, bucketID string, objectID string, text string) error {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return err
	}
	wc := client.Bucket(bucketID).Object(objectID).NewWriter(ctx)
	fmt.Fprintf(wc, text)
	if err := wc.Close(); err != nil {
		return err
	}

	return nil
}
func extractText(ctx context.Context, objectURI string) (string, error) {
	client, err := vision.NewImageAnnotatorClient(ctx)
	if err != nil {
		return "", fmt.Errorf("Failed to create client: %v", err)
	}
	defer client.Close()

	image := vision.NewImageFromURI(objectURI)
	annotations, err := client.DetectTexts(ctx, image, nil, 10)
	if err != nil {
		return "", err
	}

	if len(annotations) == 0 {
		return "", nil
	}
	return annotations[0].Description, nil
}

func main() {
	c, err := cloudevents.NewDefaultClient()
	if err != nil {
		log.Fatalf("failed to create client, %v", err)
	}
	log.Fatal(c.StartReceiver(context.Background(), receive))
}
