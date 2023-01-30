# postprocessor

## Running OwlBot locally

Follow instructions in [OwlBot Usage Guide - "How will I test my .github/.OwlBot.yaml file"](https://g3doc.corp.google.com/company/teams/cloud-client-libraries/team/automation/docs/owlbot-usage-guide.md?cl=head#how-will-i-test-my-githubowlbotyaml-file) using the instructions for
**split repositories**.
**Note**, if you replace step 2 with a clone of your own fork of the
`googleapis/googleapis-gen.git` repo, you can see how changes in your forked
`googleapis-gen` repo are eventually propagated through to the library without
making changes to the protos. Lack of permissions may also force you to clone a
fork instead of the repo.

After following these steps the generated code will have replaced corresponding
files in the `google-cloud-go` repo.

## Docker container

The Docker container needs to be built with the context of the entire
`google-cloud-go/internal` directory. When building the container, do so from
the `google-cloud-go/internal` directory

## Running the post-processor locally

The Docker container name needed will be found in the `.github/OwlBot.yaml` and
`github/OwlBot.lock.yaml` files.
To run post-processor run:

```bash
docker pull <container-name>
docker run --user $(id -u):$(id -g) --rm -v $(pwd):/repo -w /repo <container-name>
```

## Testing the post-processor locally

You can run the post-processor locally on selected directories or on all of the
clients in the root directory.

### Run post-processor on all clients

From the `google-cloud-go/internal/postprocessor` directory run:

```bash
go run main.go -stage-dir="../../owl-bot-staging/src/" -client-root="../.." -googleapis-dir="/path/to/local/googleapis"
```

### Run post-processor on select clients

From the `google-cloud-go/internal/postprocessor` directory run the same
command, but with an added `dirs` flag containing a comma-separated list of the
names of the clients on which to run the post-processor. The example below shows
the command for running the post-processor on the `accessapproval` and `asset`
libraries:

```bash
go run main.go -stage-dir="../../owl-bot-staging/src/" -client-root="../.." -googleapis-dir="/path/to/local/googleapis" -dirs="accessapproval,asset"
```
