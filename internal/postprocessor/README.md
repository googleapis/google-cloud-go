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

## Running the post-processor docker container locally

You can verify the name of the docker container name can be found in the
`.github/OwlBot.yaml` and `github/OwlBot.lock.yaml` files.

In the `google-cloud-go` root directory:

```bash
docker pull gcr.io/cloud-devrel-public-resources/owlbot-go:latest
docker run --user $(id -u):$(id -g) --rm -v $(pwd):/repo -w /repo gcr.io/cloud-devrel-public-resources/owlbot-go:latest
```

## Making changes, rebuilding the docker container and updating the OwlBot SHA

After making changes to the post-processor, you need to publish a new version
of the post-processor docker container and manually update the which version of
the post-processor is used by OwlBot. To do this you need to update the SHA in
the OwlBot lock file.

1. In your `google-cloud-go` repo, create a branch.
2. Make changes to the post-processor.
3. Test your changes. You can run the post-processor locally on selected
   clients or on all of the clients in the root directory. If the `branch`
   flag is empty/unset, the post-processor will exit early without changes.
   In the `google-cloud-go/internal/postprocessor` directory:

   ```bash
   go run . -client-root="../.." -googleapis-dir="/path/to/local/googleapis" -branch="my-branch"
   ```

   To test only selected clients:

   ```bash
   go run . -client-root="../.." -googleapis-dir="/path/to/local/googleapis" -branch="my-branch" -dirs="accessapproval,asset"
   ```
4. Clean up any changes made by post-processor test runs in the previous step.
5. Commit your changes.
6. Open your PR and respond to feedback.
7. After your PR is approved and CI is green, merge your changes. An automated
   job should update the SHA of the post-processor docker image in
   `google-cloud-go/.github/.OwlBot.lock.yaml`.

### Updating the postprocessor version used by OwlBot

After making changes to this package land in `main`, a new Docker image will be
built and pushed automatically. To update the image version used by OwlBot, run
the following commands (_you will need Docker installed and running_):

```sh
docker pull gcr.io/cloud-devrel-public-resources/owlbot-go:latest
LATEST=`docker inspect --format='{{index .RepoDigests 0}}' gcr.io/cloud-devrel-public-resources/owlbot-go:latest`
sed -i -e 's/sha256.*/'${LATEST#*@}'/g' ./.github/.OwlBot.lock.yaml
```

_Note: If run on macOS, the `sed -i` flag will need a `''` after it._

Send a pull request with the updated `.github/.OwlBot.lock.yaml`.

_Note: Any open OwlBot PR will need to be caught-up and the postprocessor rerun_
_to capture the changes._

## Initializing new modules

The post-processor initializes new modules by generating the required files
`internal/version.go`, `go.mod`, `README.md` and `CHANGES.md`.

To add a new module, add the directory name of the module to `modules` in
`google-cloud-go/internal/postprocessor/config.yaml`. Please maintain
alphabetical ordering of the module names.
