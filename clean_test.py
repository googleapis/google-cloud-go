with open('storage/grpc_writer_test.go', 'r') as f:
    content = f.read()

# Remove the comments
content = content.replace('\t\t\t// Before my fix, the logic was:\n\t\t\t// if tt.recvErr != nil { streamErr = tt.recvErr } else if tt.sendErr != nil { streamErr = tt.sendErr }\n\t\t\t// if streamErr == io.EOF { streamErr = nil }\n', '')

with open('storage/grpc_writer_test.go', 'w') as f:
    f.write(content)
