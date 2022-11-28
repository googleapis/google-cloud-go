# postprocessor

## Running OwlBot locally
Follow instructions in [OwlBot Usage Guide - "How will I test my .github/.OwlBot.yaml file"](https://g3doc.corp.google.com/company/teams/cloud-client-libraries/team/automation/docs/owlbot-usage-guide.md?cl=head#how-will-i-test-my-githubowlbotyaml-file) using the instructions for **split repositories**.
  - Note, if you replace step 2 with a clone of your own fork of the `googleapis/googleapis-gen.git` repo, you can see how changes in your forked `googleapis-gen` repo are eventually propagated through to the library without making changes to the protos. Lack of permissions may also force you to clone a fork instead of the repo.

After following these steps you should see the temporary staging directory `owl-bot-staging/` at the root of the `google-cloud-go` repo. It should contain all libraries copied from `googleapis-gen`.

## Running the post-processor locally
First, build the Docker container must be built locally and name it `postprocessor` as the `.github/.OwlBot.yaml` and `.github/.OwlBot.lock.yaml` files reference it by that name.
  - From the `google-cloud-go/internal` directory run: 
    ```sh
    docker build . -f postprocessor/Dockerfile -t postprocessor
    ```
- To run post-processor run:
    ```sh
    docker pull postprocessor
    docker run --user $(id -u):$(id -g) --rm -v $(pwd):/repo -w /repo postprocessor
    ```
    - To test that files are being copied and processed correctly, make changes in the temporary `owl-bot-staging/` directory before running the post-processor.

## Testing the post-processor locally
You can run the post-processor locally on selected directories or on all of the clients in the root directory.

### Run post-processor on all clients
From the `google-cloud-go/internal/postprocessor` directory run: 
```sh
go run main.go -stage-dir="../../owl-bot-staging/src/" -client-root="../.." -googleapis-dir="/home/guadriana/developer/googleapis"
```
### Run post-processor on select clients
From the `google-cloud-go/internal/postprocessor` directory run the same command, but with an added `dirs` flag containing a comma-separated list of the names of the clients on which to run the post-processor. The example below shows the command for running the post-processor on the `accessapproval` and `assets` libraries:
```sh
GITHUB_NAME="Adriana Gutierrez" GITHUB_USERNAME=adrianajg GITHUB_EMAIL=guadriana@google.com GITHUB_ACCESS_TOKEN=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 go run main.go -stage-dir="../../owl-bot-staging/src/" -client-root="../.." -googleapis-dir="/home/guadriana/developer/googleapis" -dirs="accessapproval,asset"
```
