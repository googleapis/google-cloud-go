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
docker pull gcr.io/cloud-devrel-public-resources/owlbot-go:infrastructure-public-image-latest
docker run --user $(id -u):$(id -g) --rm -v $(pwd):/repo -w /repo gcr.io/cloud-devrel-public-resources/owlbot-go:infrastructure-public-image-latest
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
the following command (_you will need Docker installed and running_):

```sh
docker pull gcr.io/cloud-devrel-public-resources/owlbot-go:infrastructure-public-image-latest
```

Extract the `sha256` Digest from the logs emitted by the `pull` and set it as
the digest in the [lockfile](../../.github/.OwlBot.lock.yaml).

Send a pull request with the updated `.github/.OwlBot.lock.yaml`.

_Note: Any open OwlBot PR will need to be caught-up and the postprocessor rerun_
_to capture the changes._

## Initializing new modules

The post-processor initializes new modules by generating the required files
`internal/version.go`, `go.mod`, `README.md` and `CHANGES.md`.

To add a new module, add the directory name of the module to `modules` in
`google-cloud-go/internal/postprocessor/config.yaml`. Please maintain
alphabetical ordering of the module names.

## Validating your config changes

The `validate` command is run as a presubmit on changes to either the
`.github/.OwlBot.yaml` or the `internal/postprocessor/config.yaml`.

If you want to run it manually, from the **repository root**, simply run the
following:

```
go run ./internal/postprocessor validate
```

If you want to validate existence of service config yaml in the PostProcessor
config, provide an absolute path to a local clone of `googleapis`:

```
go run ./internal/postprocessor validate -googleapis-dir=$GOOGLEAPIS
```

If you want validate a specific config file, not the repository default, then
provide aboslute paths to either or both config files like so:

```
go run ./internal/postprocessor \
   -owl-bot-config=$OWL_BOT_YAML \
   -processor-config=$CONFIG_YAML
```

If you think there is an issue with the validator, just fix it in the same CL
as the config change that triggered it. No need to update the postprocessor sha
when the validate command is changed, it runs from HEAD of the branch.