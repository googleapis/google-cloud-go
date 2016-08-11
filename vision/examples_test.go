package vision_test

import (
	"fmt"
	"os"

	"cloud.google.com/go/vision"
	"golang.org/x/net/context"
)

func ExampleNewClient() {
	ctx := context.Background()
	client, err := vision.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	// Use the client.

	// Close the client when finished.
	if err := client.Close(); err != nil {
		// TODO: handle error.
	}
}

func Example_NewImageFromReader() {
	f, err := os.Open("path/to/image.jpg")
	if err != nil {
		// TODO: handle error.
	}
	img, err := vision.NewImageFromReader(f)
	if err != nil {
		// TODO: handle error.
	}
	fmt.Println(img)
}

func Example_NewImageFromGCS() {
	img := vision.NewImageFromGCS("gs://my-bucket/my-image.png")
	fmt.Println(img)
}

func ExampleClient_Annotate_oneImage() {
	ctx := context.Background()
	client, err := vision.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	annsSlice, err := client.Annotate(ctx, &vision.AnnotateRequest{
		Image:      vision.NewImageFromGCS("gs://my-bucket/my-image.png"),
		MaxLogos:   100,
		MaxTexts:   100,
		SafeSearch: true,
	})
	if err != nil {
		// TODO: handle error.
	}
	anns := annsSlice[0]
	if anns.Logos != nil {
		fmt.Println(anns.Logos)
	}
	if anns.Texts != nil {
		fmt.Println(anns.Texts)
	}
	if anns.SafeSearch != nil {
		fmt.Println(anns.SafeSearch)
	}
	if anns.Error != nil {
		fmt.Printf("at least one of the features failed: %v", anns.Error)
	}
}

func ExampleClient_DetectFaces() {
	ctx := context.Background()
	client, err := vision.NewClient(ctx)
	if err != nil {
		// TODO: handle error.
	}
	img := vision.NewImageFromGCS("gs://my-bucket/my-image.png")
	faces, err := client.DetectFaces(ctx, img, 10)
	if err != nil {
		// TODO: handle error.
	}
	fmt.Println(faces[0].Face.Nose.Tip)
	fmt.Println(faces[0].Face.Eyes.Left.Pupil)
}
