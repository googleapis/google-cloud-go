# Uses the sentencepiece package to tokenize the file provided as a command-line
# argument; emits all token IDs to stdout, one per line.
#
# Requires the MODELPATH env var to be set to the binary proto describing
# the tokenizer model.
import sentencepiece as spm
import os, sys

with open(sys.argv[1], "r", newline="") as f:
    text = f.read()
    sp = spm.SentencePieceProcessor(model_file=os.getenv("MODELPATH"))
    ids = sp.encode(text)

    # Print ids out, one per line
    for id in ids:
        print(id)
