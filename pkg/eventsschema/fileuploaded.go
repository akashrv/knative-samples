package eventsschema

// FileUploaded defines the Data of CE with type=custom.fileuploaded
type FileUploaded struct {
	ExplicitContent bool   `json:"explicitcontent,omitempty,string"`
	BucketID        string `json:"bucketid,omitempty"`
	ObjectID        string `json:"objectid,omitempty"`
}
