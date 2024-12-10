// Copyright 2024 Google LLC
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

package main

// Command dumper is a debugging utility for internal use. It helps explore
// the model proto and compare results with other tools.

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"unicode"

	"cloud.google.com/go/vertexai/internal/sentencepiece"
	"cloud.google.com/go/vertexai/internal/sentencepiece/internal/model"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
)

func main() {
	fDumpAll := flag.Bool("dumpall", false, "dump entire model proto")
	fFindUni := flag.Bool("finduni", false, "find unicode runes not in pieces")
	fFindBytes := flag.Bool("findbytes", false, "show all byte pieces with their IDs")
	fEncodeFile := flag.String("encodefile", "", "file name to open and encode")
	flag.Parse()

	modelPath := os.Getenv("MODELPATH")
	if modelPath == "" {
		log.Fatal("Need MODELPATH env var to run")
	}

	b, err := ioutil.ReadFile(modelPath)
	if err != nil {
		log.Fatal(err)
	}

	var protomodel model.ModelProto
	err = proto.Unmarshal(b, &protomodel)
	if err != nil {
		log.Fatal(err)
	}

	if *fDumpAll {
		fmt.Println(prototext.Format(&protomodel))
	} else if *fFindBytes {
		for i, piece := range protomodel.GetPieces() {
			if piece.GetType() == model.ModelProto_SentencePiece_BYTE {
				fmt.Printf("%5d: %s\n", i, piece.GetPiece())
			}
		}

	} else if *fFindUni {
		pieces := make(map[string]int)
		for i, piece := range protomodel.GetPieces() {
			pieces[piece.GetPiece()] = i
		}

		for r := rune(0); r <= unicode.MaxRune; r++ {
			if unicode.IsPrint(r) {
				if _, found := pieces[string(r)]; !found {
					fmt.Printf("not in pieces: %U %q\n", r, string(r))
				}
			}
		}
	} else if *fEncodeFile != "" {
		proc, err := sentencepiece.NewProcessorFromPath(modelPath)
		if err != nil {
			log.Fatal(err)
		}

		b, err := ioutil.ReadFile(*fEncodeFile)
		if err != nil {
			log.Fatal(err)
		}

		tokens := proc.Encode(string(b))
		for _, t := range tokens {
			fmt.Println(t.ID)
		}
	}
}
