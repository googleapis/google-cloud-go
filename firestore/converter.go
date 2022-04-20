package firestore

type FirestoreConverter interface {
	ToFirestore() (interface{}, error)
	FromFirestore(v interface{}) error
}
