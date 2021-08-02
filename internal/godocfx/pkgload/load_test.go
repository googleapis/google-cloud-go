package pkgload

import "testing"

func TestPkgStatus(t *testing.T) {
	tests := []struct {
		importPath string
		doc        string
		want       string
	}{
		{
			importPath: "cloud.google.com/go",
			want:       "",
		},
		{
			importPath: "cloud.google.com/go/storage/v1alpha1",
			want:       "alpha",
		},
		{
			importPath: "cloud.google.com/go/storage/v2beta2",
			want:       "beta",
		},
		{
			doc:  "NOTE: This package is in beta. It is not stable, and may be subject to changes.",
			want: "beta",
		},
		{
			doc:  "NOTE: This package is in alpha. It is not stable, and is likely to change.",
			want: "alpha",
		},
		{
			doc:  "Package foo is great\nDeprecated: not anymore",
			want: "deprecated",
		},
		{
			importPath: "cloud.google.com/go/storage/v1alpha1",
			doc:        "Package foo is great\nDeprecated: not anymore",
			want:       "deprecated", // Deprecated comes before alpha and beta.
		},
		{
			importPath: "cloud.google.com/go/storage/v1beta1",
			doc:        "Package foo is great\nDeprecated: not anymore",
			want:       "deprecated", // Deprecated comes before alpha and beta.
		},
	}
	for _, test := range tests {
		if got := pkgStatus(test.importPath, test.doc); got != test.want {
			t.Errorf("pkgStatus(%q, %q) got %q, want %q", test.importPath, test.doc, got, test.want)
		}
	}
}
