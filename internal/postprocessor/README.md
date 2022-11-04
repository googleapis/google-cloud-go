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
From the `google-cloud-go/internal/postprocessor` directory run: 
```sh
go run main.go -src="../../owl-bot-staging/src/" -dst="../.." -testing=True
```
