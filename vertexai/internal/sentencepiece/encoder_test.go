package sentencepiece

import (
	"os"
	"slices"
	"testing"
)

func createEncoder(t testing.TB) *Encoder {
	t.Helper()
	protoFile := os.Getenv("MODELPATH")
	if protoFile == "" {
		t.Fatal("Need MODELPATH env var to run tests")
	}

	encoder, err := NewEncoderFromPath(protoFile)
	if err != nil {
		t.Error(err)
	}
	return encoder
}

func TestEncodeIDs(t *testing.T) {
	enc := createEncoder(t)

	var tests = []struct {
		text    string
		wantIDs []int
	}{
		{"hello world", []int{17534, 2134}},
		{"12345", []int{235274, 235284, 235304, 235310, 235308}},
		{"  ", []int{139}},
		{"   ", []int{140}},
		{"        ", []int{145}},
		{"ҔӌԐڎ", []int{427, 365, 428, 357, 429, 361, 435, 359}},
		{" <mask>  <pad>", []int{235248, 4, 139, 235322, 8939, 235313}},
		{"<table><th></th></table>", []int{169, 175, 183, 177}},
		{"one line\nand another line", []int{785, 2017, 108, 639, 2550, 2017}},
		{"Language: English\r\n\r\nCredits: Produced by David Widger\r\n", []int{14357, 235292, 4645, 235316, 108, 235316, 108, 34711, 235292, 99662, 731, 6046, 37303, 1197, 235316, 108}},
		{"Bienvenido a este proyecto", []int{176831, 476, 4004, 25431}},
		{"अस्मिन् परियोजनायां स्वागतम्", []int{236088, 22740, 212361, 18029, 14480, 19900, 146166, 6751, 235563, 56545, 44071, 235550, 26989}},
		{"if allow == true { return x;} else {return x+y;}", []int{648, 2765, 1159, 1382, 612, 2203, 1141, 22505, 1354, 612, 773, 1141, 235340, 235267, 22505}},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got := enc.Encode(tt.text)

			var gotIDs []int
			for _, t := range got {
				gotIDs = append(gotIDs, t.ID)
			}

			if !slices.Equal(gotIDs, tt.wantIDs) {
				t.Errorf("got  %v\nwant: %v\n", gotIDs, tt.wantIDs)
			}
		})
	}
}

func TestEncodeWithText(t *testing.T) {
	enc := createEncoder(t)

	var tests = []struct {
		text       string
		wantTokens []Token
	}{
		{"hi <td> bye",
			[]Token{
				{544, "hi"},
				{235248, "▁"},
				{176, "<td>"},
				{44788, "▁bye"},
			}},
		{"hiƻ <td>🤨there ⇲bob, สวัสดี",
			[]Token{
				{544, "hi"},
				{415, "<0xC6>"},
				{404, "<0xBB>"},
				{235248, "▁"},
				{176, "<td>"},
				{241847, "🤨"},
				{11048, "there"},
				{235248, "▁"},
				{248372, "⇲"},
				{26242, "bob"},
				{235269, ","},
				{12515, "▁ส"},
				{151622, "วัส"},
				{28890, "ดี"},
			}},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got := enc.Encode(tt.text)
			if !slices.Equal(got, tt.wantTokens) {
				t.Errorf("got  %v\nwant: %v\n", got, tt.wantTokens)
			}
		})
	}
}

func TestSymbolMatch(t *testing.T) {
	enc := createEncoder(t)

	var tests = []struct {
		text      string
		wantLen   int
		wantFound bool
	}{
		{"<td>", 4, true},
		{"<s>", 3, true},
		{"</s>", 4, true},
		{"<start_of_turn>", 15, true},
		{"<start_of_turn!", 1, false},
		{"▁▁", 6, true},
		{"▁▁▁▁▁▁", 18, true},
		{"bob", 1, false},
		{"🤨", 4, false},
		{"สวัสดี", 3, false},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			gotLen, gotFound := enc.symbolMatch(tt.text)
			if gotLen != tt.wantLen || gotFound != tt.wantFound {
				t.Errorf("got (%v, %v), want (%v, %v)", gotLen, gotFound, tt.wantLen, tt.wantFound)
			}
		})
	}
}

func TestConvertHexValue(t *testing.T) {
	var tests = []struct {
		in    string
		wantN int
	}{
		{"<0x40>", 64},
		{"<0x00>", 0},
		{"<0x1a>", 26},
		{"<0xF3>", 243},

		{"0x12>", -1},
		{"<x12>", -1},
		{"<012>", -1},
		{"<0xTA>", -1},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			gotN := convertHexValue(tt.in)
			if gotN != tt.wantN {
				t.Errorf("got %v, want %v", gotN, tt.wantN)
			}
		})
	}
}
