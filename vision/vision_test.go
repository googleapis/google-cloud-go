// Copyright 2016 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package vision

import (
	"fmt"
	"os"
	"testing"

	"cloud.google.com/go/internal/testutil"
	"golang.org/x/net/context"
	"google.golang.org/api/option"
)

func TestAnnotate(t *testing.T) {
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	tests := []struct {
		path string // path to image file, relative to testdata
		// If one of these is true, we expect that annotation to be non-nil.
		faces, landmarks, logos, labels, texts bool
		// We always expect safe search and image properties to be present.
	}{
		{path: "face_detection/face.jpg", faces: true, labels: true},
		{path: "label/cat.jpg", labels: true},
		{path: "label/faulkner.jpg", labels: true},
		{path: "text/mountain.jpg", texts: true, labels: true},
		{path: "text/no-text.jpg", labels: true},
	}
	for _, test := range tests {
		img, err := readTestImage(test.path)
		if err != nil {
			t.Fatal(err)
		}
		annsSlice, err := client.Annotate(ctx, &AnnotateRequest{
			Image:           img,
			MaxFaces:        1,
			MaxLandmarks:    1,
			MaxLogos:        1,
			MaxLabels:       1,
			MaxTexts:        1,
			SafeSearch:      true,
			ImageProperties: true,
		})
		if err != nil {
			t.Fatalf("annotating %s: %v", test.path, err)
		}
		anns := annsSlice[0]
		p := map[bool]string{true: "present", false: "absent"}
		if got, want := (anns.Faces != nil), test.faces; got != want {
			t.Errorf("%s: faces %s, want %s", test.path, p[got], p[want])
		}
		if got, want := (anns.Landmarks != nil), test.landmarks; got != want {
			if anns.Landmarks != nil {
				fmt.Printf("%s: landmarks %+v\n", test.path, anns.Landmarks[0])
			}
			t.Errorf("%s: landmarks %s, want %s", test.path, p[got], p[want])
		}
		if got, want := (anns.Logos != nil), test.logos; got != want {
			if anns.Logos != nil {
				fmt.Printf("%s: logos %+v\n", test.path, anns.Logos[0])
			}
			t.Errorf("%s: logos %s, want %s", test.path, p[got], p[want])
		}
		if got, want := (anns.Labels != nil), test.labels; got != want {
			t.Errorf("%s: labels %s, want %s", test.path, p[got], p[want])
		}
		if got, want := (anns.Texts != nil), test.texts; got != want {
			t.Errorf("%s: texts %s, want %s", test.path, p[got], p[want])
		}
		if got, want := (anns.SafeSearch != nil), true; got != want {
			t.Errorf("%s: safe search %s, want %s", test.path, p[got], p[want])
		}
		if got, want := (anns.ImageProperties != nil), true; got != want {
			t.Errorf("%s: image properties %s, want %s", test.path, p[got], p[want])
		}
		if anns.Error != nil {
			t.Errorf("%s: got Error %v; want nil", test.path, anns.Error)
		}
	}
}

// Test the single-image single-feature methods, like DetectFaces.
func TestDetectionMethods(t *testing.T) {
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	for _, test := range []struct {
		path string
		call func(*Image) (bool, error)
	}{
		{"face_detection/face.jpg",
			func(img *Image) (bool, error) {
				as, err := client.DetectFaces(ctx, img, 1)
				return as != nil, err
			},
		},
	} {
		img, err := readTestImage(test.path)
		if err != nil {
			t.Fatalf("%s: %v", test.path, err)
		}
		present, err := test.call(img)
		if err != nil {
			t.Errorf("%s: got err %v, want nil", test.path, err)
		}
		if !present {
			t.Errorf("%s: nil annotation, want non-nil", test.path)
		}
	}
}

func TestErrors(t *testing.T) {
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	// Empty image.
	// With Client.Annotate, the RPC succeeds, but the Error field is non-nil.
	_, err := client.Annotate(ctx, &AnnotateRequest{
		Image:           &Image{},
		ImageProperties: true,
	})
	if err != nil {
		t.Errorf("got %v, want nil", err)
	}

	// With a Client.DetectXXX method, the Error field becomes the return value.
	_, err = client.DetectFaces(ctx, &Image{}, 1)
	if err == nil {
		t.Error("got nil, want error")
	}

	// Invalid image.
	badImg := &Image{content: []byte("ceci n'est pas une image")}
	// If only ImageProperties is specified, the result is an annotation
	// with all fields (including Error) nil. But any actual detection will fail.
	_, err = client.Annotate(ctx, &AnnotateRequest{
		Image:      badImg,
		SafeSearch: true,
	})
	if err != nil {
		t.Errorf("got %v, want error", err)
	}

	// With a Client.DetectXXX method, the Error field becomes the return value.
	_, err = client.DetectFaces(ctx, &Image{}, 1)
	if err == nil {
		t.Error("got nil, want error")
	}
}

func integrationTestClient(ctx context.Context, t *testing.T) *Client {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	ts := testutil.TokenSource(ctx, Scope)
	if ts == nil {
		t.Skip("Integration tests skipped. See CONTRIBUTING.md for details")
	}
	client, err := NewClient(ctx, option.WithTokenSource(ts))
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func readTestImage(path string) (*Image, error) {
	f, err := os.Open("testdata/" + path)
	if err != nil {
		return nil, err
	}
	img, err := NewImageFromReader(f)
	if err != nil {
		return nil, fmt.Errorf("%q: %v", path, err)
	}
	return img, nil
}
