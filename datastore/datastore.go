package datastore

type Dataset struct {
	ID string // project ID

	// TODO(jbd): Connection should be represented by oauth2
	// credentials.
}

func NewDataset(projectID string) *Dataset {
	return &Dataset{ID: projectID}
}

// TODO: Add querying

func (d *Dataset) Get(key *Key, dst interface{}) error {
	panic("not yet implemented")
}

func (d *Dataset) Put(key *Key, src interface{}) (*Key, error) {
	panic("not yet implemented")
}

func (d *Dataset) Delete(key *Key) error {
	panic("not yet implemented")
}

func (d *Dataset) AllocateIDs(kind string, n int) error {
	panic("not yet implemented")
}

func (d *Dataset) RunInTransaction(fn func() error) error {
	panic("not yet implemented")
}
