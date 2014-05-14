package datastore

import (
	"github.com/googlecloudplatform/gcloud-golang/datastore/pb"
)

func keyToPbKey(k *Key) *pb.Key {
	// TODO(jbd): Panic if dataset ID is not provided.
	pathEl := &pb.Key_PathElement{Kind: &k.kind}
	if k.intID > 0 {
		pathEl.Id = &k.intID
	}
	if k.name != "" {
		pathEl.Name = &k.name
	}
	return &pb.Key{
		PartitionId: &pb.PartitionId{
			DatasetId: &k.datasetID,
			Namespace: &k.namespace,
		},
		PathElement: []*pb.Key_PathElement{pathEl},
	}
}

// TODO(jbd): Minimize reflect, cache conversion method for
// known types.
func entityToPbEntity(src interface{}) *pb.Entity {
	panic("not yet implemented")
	return &pb.Entity{}
}

func entityFromPbEntry(e *pb.Entity, dst interface{}) {
	panic("not yet implemented")
}
