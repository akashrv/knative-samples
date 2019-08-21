package main

import (
	"context"
	"fmt"
	"net/http"

	// To do move to zap logger
	"log"

	visionapi "cloud.google.com/go/vision/apiv1"
	cloudevents "github.com/cloudevents/sdk-go"
	visionproto "google.golang.org/genproto/googleapis/cloud/vision/v1"
)

var (
	finalizeEventType = "google.storage.object.finalize"
)

func receive(ctx context.Context, event cloudevents.Event, response *cloudevents.EventResponse) error {
	if isExplicit, status, err := processEvent(ctx, event); err != nil {
		response.Error(status, err.Error())
		log.Println("Error occured:", err.Error())
	} else {
		fileUploadedEvent := cloudevents.NewEvent()
		fileUploadedEvent.Context = event.Context.Clone()
		fileUploadedEvent.Context.SetType("custom.fileuploaded")
		fileUploadedEvent.Context.SetExtension("explicitcontent", isExplicit)
		fileUploadedEvent.SetData(event.Data)
		fileUploadedEvent.DataEncoded = event.DataEncoded
		response.RespondWith(status, &fileUploadedEvent)
	}
	return nil
}

func processEvent(ctx context.Context, event cloudevents.Event) (bool, int, error) {
	// do something with event.Context and event.Data (via event.DataAs(foo)
	if event.Context == nil {
		return false, http.StatusBadRequest, fmt.Errorf("event.Context is nil. cloudevents.Event\n%s", event.String())
	}

	if event.Context.GetType() != finalizeEventType {
		return false, http.StatusBadRequest, fmt.Errorf("invalid event type %s. Supported event type: %s", event.Context.GetType(), finalizeEventType)
	}

	extensionAttributes := event.Context.GetExtensions()
	bucketID, err := getBucketID(extensionAttributes)
	if err != nil {
		return false, http.StatusBadRequest, err
	}
	objectID, err := getObjectID(extensionAttributes)
	if err != nil {
		return false, http.StatusBadRequest, err
	}
	objectURI := getObjectURI(bucketID, objectID)
	log.Println("ObjectURI:", objectURI)

	isExplicit, err := isContentExplicit(ctx, objectURI)
	if err != nil {
		return false, http.StatusInternalServerError, err
	}
	return isExplicit, http.StatusOK, nil
}

func getBucketID(extensions map[string]interface{}) (string, error) {
	bucketID, ok := extensions["bucketid"]
	if !ok {
		return "", fmt.Errorf("bucketid extension attribute not found")
	}
	bucketIDString, ok := bucketID.(string)
	if !ok {
		return "", fmt.Errorf("bucketID %v cannot be converted to string", bucketID)
	}
	return bucketIDString, nil

}

func getObjectID(extensions map[string]interface{}) (string, error) {
	objectID, ok := extensions["objectid"]
	if !ok {
		return "", fmt.Errorf("objectid extension attribute not found")
	}
	objectIDString, ok := objectID.(string)
	if !ok {
		return "", fmt.Errorf("objectID %v cannot be converted to string", objectID)
	}
	return objectIDString, nil

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
