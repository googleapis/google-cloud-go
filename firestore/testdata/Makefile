# Copy textproto files in this directory from the source of truth.

SRC=$(GOPATH)/src/github.com/GoogleCloudPlatform/google-cloud-common/testing/firestore/testdata

refresh:
	-rm *.textproto
	cp $(SRC)/*.textproto .
