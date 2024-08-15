package sentencepiece

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
)

// "System" test for comparing our Encoder with the canonical sentencepiece
// Python package (officially distributed with the original C++ implementation
// of the algorithm).
//
// This test will only run if python3 is available and is able to successfully
// load the sentencepiece library. Typically this means that 'go test' will
// have to run from an activated Python virtual environment where the library
// was installed.

func TestVsSentencepiecePython(t *testing.T) {
	enc := createEncoder(t)

	if _, err := exec.Command("python3", "-c", "import sentencepiece").Output(); err != nil {
		t.Skip("This test only runs when python3 with sentencepiece is available")
	}
	pyProgramPath := filepath.Join("test", "sp-dump-ids.py")

	paths, err := filepath.Glob(filepath.Join("test", "*.txt"))
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range paths {
		_, filename := filepath.Split(path)
		testname := filename[:len(filename)-len(filepath.Ext(path))]

		t.Run(testname, func(t *testing.T) {
			// Step 1: run the Python program to tokenize path into IDs.
			pyOut, err := exec.Command("python3", pyProgramPath, path).Output()
			if err != nil {
				t.Fatalf("while running %v on %v: %v", pyProgramPath, path, err)
			}

			pyIDs := pyOutToIDs(pyOut)

			// Step 2: use our Encoder to tokenize path into IDs.
			buf, err := ioutil.ReadFile(path)
			if err != nil {
				log.Fatal(err)
			}
			var goIDs []int
			goTokens := enc.Encode(string(buf))
			for _, t := range goTokens {
				goIDs = append(goIDs, t.ID)
			}

			// Step 3: compare the two; dump IDs to temp files for debugging in case
			// of a mismatch.
			if !slices.Equal(pyIDs, goIDs) {
				tmppy := dumpIDsToTempFile(testname+"-py-", pyIDs)
				tmpgo := dumpIDsToTempFile(testname+"-go-", goIDs)

				t.Errorf("IDs mismatch; dumped to %q and %q", tmppy, tmpgo)
			}
		})
	}
}

// pyOutToIDs takes the entire stdout output of the Python program and parses
// it into a list of integer IDs.
func pyOutToIDs(pyOut []byte) []int {
	var IDs []int
	scanner := bufio.NewScanner(bytes.NewReader(pyOut))
	for scanner.Scan() {
		i, err := strconv.Atoi(scanner.Text())
		if err != nil {
			log.Fatal(err)
		}
		IDs = append(IDs, i)
	}
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
	return IDs
}

// dumpIDsToTempFile dumps the given IDs (one per line) to a temporary file with
// the given prefix, and returns the name of the temporary file.
func dumpIDsToTempFile(prefix string, IDs []int) string {
	tf, err := os.CreateTemp("", prefix)
	if err != nil {
		log.Fatal(err)
	}
	defer tf.Close()

	for _, id := range IDs {
		fmt.Fprintf(tf, "%d\n", id)
	}
	return tf.Name()
}
