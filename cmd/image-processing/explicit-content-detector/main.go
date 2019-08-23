package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/akashrv/knative-samples/pkg/eventsschema"

	// To do move to zap logger
	"log"

	visionapi "cloud.google.com/go/vision/apiv1"
	cloudevents "github.com/cloudevents/sdk-go"
	"github.com/google/uuid"
	"google.golang.org/api/storage/v1"
	visionproto "google.golang.org/genproto/googleapis/cloud/vision/v1"
)

var (
	finalizeEventType = "google.storage.object.finalize"
)

func receive(ctx context.Context, event cloudevents.Event, response *cloudevents.EventResponse) error {
	if fileUploadedEventData, status, err := processEvent(ctx, event); err != nil {
		response.Error(status, err.Error())
		log.Println("Error occured:", err.Error())
	} else {
		fileUploadedEvent := cloudevents.NewEvent()
		fileUploadedEvent.Context.SetID(uuid.New().String())
		fileUploadedEvent.Context.SetSource("custom.explicit-content-detector")
		fileUploadedEvent.Context.SetSpecVersion(cloudevents.VersionV03)
		fileUploadedEvent.Context.SetType("custom.fileuploaded")
		if fileUploadedEventData.ExplicitContent {
			fileUploadedEvent.Context.SetExtension("customextension", "explicit-content")
		} else {
			fileUploadedEvent.Context.SetExtension("customextension", "no-explicit-content")
		}

		fileUploadedEvent.SetData(fileUploadedEventData)
		response.RespondWith(status, &fileUploadedEvent)
		log.Printf("Reponded with event %v", fileUploadedEvent)
	}
	return nil
}

func processEvent(ctx context.Context, event cloudevents.Event) (*eventsschema.FileUploaded, int, error) {
	// do something with event.Context and event.Data (via event.DataAs(foo)
	if event.Context == nil {
		return nil, http.StatusBadRequest, fmt.Errorf("event.Context is nil. cloudevents.Event\n%s", event.String())
	}

	if event.Context.GetType() != finalizeEventType {
		return nil, http.StatusBadRequest, fmt.Errorf("invalid event type %s. Supported event type: %s", event.Context.GetType(), finalizeEventType)
	}

	data := storage.Object{}
	err := event.DataAs(&data)

	if err != nil {
		log.Println("Error converting event.Data to google.golang.org/api/storage/v1/storage.object.", err.Error())
	}

	objectURI := getObjectURI(data.Bucket, data.Name)
	log.Println("ObjectURI:", objectURI)

	isExplicit, err := isContentExplicit(ctx, objectURI)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	return &eventsschema.FileUploaded{
		BucketID:        data.Bucket,
		ObjectID:        data.Name,
		ExplicitContent: isExplicit,
	}, http.StatusOK, nil
}

func isContentExplicit(ctx context.Context, objectURI string) (bool, error) {

	client, err := visionapi.NewImageAnnotatorClient(ctx)
	if err != nil {
		return false, fmt.Errorf("Failed to create client: %v", err)
	}
	defer client.Close()

	image := visionapi.NewImageFromURI(objectURI)

	props, err := client.DetectSafeSearch(ctx, image, nil)
	if err != nil {
		return false, err
	}
	if props.GetAdult() >= visionproto.Likelihood_LIKELY ||
		props.GetMedical() >= visionproto.Likelihood_LIKELY ||
		props.GetRacy() >= visionproto.Likelihood_LIKELY ||
		props.GetSpoof() >= visionproto.Likelihood_LIKELY ||
		props.GetViolence() >= visionproto.Likelihood_LIKELY {
		return true, nil
	}
	return false, nil
}
func getObjectURI(bucketID string, objectID string) string {
	return fmt.Sprintf("gs://%s/%s", bucketID, objectID)
}

func main() {
	c, err := cloudevents.NewDefaultClient()
	if err != nil {
		log.Fatalf("failed to create client, %v", err)
	}
	log.Fatal(c.StartReceiver(context.Background(), receive))
}
