# How to Contribute

We'd love to accept your patches and contributions to this project. There are
just a few small guidelines you need to follow.

## Contributor License Agreement

Contributions to this project must be accompanied by a Contributor License
Agreement. You (or your employer) retain the copyright to your contribution;
this simply gives us permission to use and redistribute your contributions as
part of the project. Head over to
[https://cla.developers.google.com/](https://cla.developers.google.com/) to see
your current agreements on file or to sign a new one.  You generally only need
to submit a CLA once, so if you've already submitted one (even if it was for a
different project), you probably don't need to do it again.

## Getting Started

1. [File an issue](https://github.com/googleapis/google-cloud-go/issues/new/choose).
   The issue will be used to discuss the bug or feature and should be created
   before sending a PR.

1. [Install Go](https://golang.org/dl/).
    1. Ensure that your `GOBIN` directory (by default `$(go env GOPATH)/bin`)
    is in your `PATH`.
    1. Check it's working by running `go version`.
        * If it doesn't work, check the install location, usually
        `/usr/local/go`, is on your `PATH`.

1. Sign one of the
[contributor license agreements](#contributor-license-agreements) below.

1. Clone the repo:
    `git clone https://github.com/googleapis/google-cloud-go`

1. Change into the checked out source:
    `cd google-cloud-go`

1. Fork the repo.

1. Set your fork as a remote:
    `git remote add fork git@github.com:GITHUB_USERNAME/google-cloud-go.git`

1. Make changes, commit to your fork.

   Commit messages should follow the
   [Conventional Commits Style](https://www.conventionalcommits.org). The scope
   portion should always be filled with the name of the package affected by the
   changes being made. For example:
   ```
   feat(functions): add gophers codelab
   ```

1. Send a pull request with your changes.

   To minimize friction, consider setting `Allow edits from maintainers` on the
   PR, which will enable project committers and automation to update your PR.

1. A maintainer will review the pull request and make comments.

   Prefer adding additional commits over amending and force-pushing since it can
   be difficult to follow code reviews when the commit history changes.

   Commits will be squashed when they're merged.

## Code Reviews

All submissions, including submissions by project members, require review. We
use GitHub pull requests for this purpose. Submissions by non-Googlers require
two reviewers. Consult [GitHub
Help](https://help.github.com/articles/about-pull-requests/) for more
information on using pull requests.

### Multi-Approvers Check

Each pull request must be approved by two Googlers. This is enforced by an
automated multi-approvers check. This check may not automatically re-run after
the second approval is added. If it remains in a failed state, you can manually
re-trigger it by:
- Clicking "View details" on the failed workflow to bring you to the "Actions"
  page.
- Clicking "Re-run failed jobs".

For more information, see
[Re-running failed jobs in a workflow](https://docs.github.com/en/actions/how-tos/managing-workflow-runs-and-deployments/managing-workflow-runs/re-running-workflows-and-jobs#re-running-failed-jobs-in-a-workflow).

## Community Guidelines

This project follows
[Google's Open Source Community Guidelines](https://opensource.google/conduct/).

## Before contributing code

Before doing any significant work, open an issue to propose your idea and
ensure alignment. You can either
[file a new issue](https://github.com/googleapis/google-cloud-go/issues/new/choose), or comment on an
[existing one](https://github.com/googleapis/google-cloud-go/issues).

A pull request (PR) that does not go through this coordination process may be
closed to avoid wasted effort.  Make sure your code follows the
[style guidelines](ARCHITECTURE.md).

## Using the issue tracker

We use GitHub issues to track tasks, bugs, and discussions. Use the issue
tracker as your source of truth.

## Filing a new issue

All changes, except trivial ones, should start with a GitHub issue.

This process gives everyone a chance to validate the design, helps prevent
duplication of effort, and ensures that the idea fits inside the goals for the
language and tools. It also checks that the design is sound before code is
written; the code review tool is not the place for high-level discussions.
Always include a clear description in the body of the issue. The description
should provide enough context for any team member to understand the problem or
request without needing to contact you directly for clarification.

## Leaving a TODO

When adding a TODO to the codebase, always include a link to an issue, no
matter how small the task.

Use the format:

```
// TODO(https://github.com/googleapis/google-cloud-go/issues/<number>): explain what needs to be done
```

This helps provide context for future readers and keeps the TODO relevant and
actionable as the project evolves.

## Sending a pull request

All code changes must be submitted via a pull request. If you are a first-time
contributor, please review the
[GitHub flow](https://docs.github.com/en/get-started/using-github/github-flow)
before starting.

Before sending a pull request, make sure it includes tests if there are logic
changes, copyright headers in every file, and a commit message following the
conventions in the
[Commit messages](#commit-messages)
section below.

### Open pull requests from a personal fork

Open pull requests from a personal fork. When opening your pull request, enable
"Allow edits from maintainers" to allow others to help you with minor tweaks or
merge conflicts directly.

For a step-by-step guide, see the official documentation on
[creating a pull request from a fork](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/proposing-changes-to-your-work-with-pull-requests/creating-a-pull-request-from-a-fork).

### Keep pull requests up to date with base branch

The repository is configured to not require branches to be up to date before
merging. This means that you do not have to have the latest changes from the
base branch integrated, unless GitHub detects merge conflicts.  To minimize the
risk of the pull request getting out of date with the base branch, enable
[auto-merge](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/incorporating-changes-from-a-pull-request/automatically-merging-a-pull-request)
so that the pull request submits as soon as it is approved and the checks pass.

## Commit messages

Commit messages should follow the conventions below:

Here is an example:

```
feat(storage): add new storage bucket feature

A new feature is added to storage.

Fixes \#238
```

### First line

The first line of the change description is a short one-line summary of the
change, following the structure `<type>(<package>): <description>`:

#### type

A structural element defined by the conventions at
[https://www.conventionalcommits.org/en/v1.0.0/\#summary](https://www.conventionalcommits.org/en/v1.0.0/#summary).

Conventional commits are parsed by release tooling to generate release notes.

#### package

The name of the package affected by the change, and should be provided in
parentheses before the colon. (For example, storage or pubsub).

### description

A short one-line summary of the change, which should be written to
complete the sentence "This change modifies the crate to ..." That means it
does not start with a capital letter, is not a complete sentence, and actually
summarizes the result of the change. Note that the verb after the colon is
lowercase, and there is no trailing period.  The first line should be kept as
short as possible (many git viewing tools prefer under ~76 characters).

Follow the first line by a blank line.

### Main content

The rest of the commit message should provide context for the change and
explain what it does. Write in complete sentences with correct punctuation.
Don't use HTML, Markdown, or any other markup language.

### Referencing issues

The special notation "Fixes \#12345" associates the change with issue 12345 in
the issue tracker. When this change is eventually applied, the issue tracker
will automatically mark the issue as fixed.  If the change is a partial step
towards the resolution of the issue, write "For \#12345" instead. This will
leave a comment in the issue linking back to the pull request, but it will not
close the issue when the change is applied.  Please don’t use alternate
GitHub-supported aliases like Close or Resolves instead of Fixes.

## The review process

This section explains the review process in detail and how to approach reviews
after a pull request has been sent for review.

### Getting a code review

Before creating a pull request, make sure that your commit message follows the
suggested format. Otherwise, it can be common for the pull request to be sent
back with that request without review.  After creating a pull request, request
a specific reviewer if relevant, or leave it for the default group.

### Merging a pull request

Pull request titles and descriptions must follow the
[commit messages](#commit-messages)
conventions. This enables approvers to review the final commit message. Once
the pull request has been approved and all checks have passed, click the
[Squash and Merge](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/incorporating-changes-from-a-pull-request/about-pull-request-merges#squash-and-merge-your-commits)
button. The resulting commit message will be based on the pull request's title
and description.

### Reverting a pull request

If a merged pull request needs to be undone, for reasons such as breaking the build, the standard process is to
[revert it through the GitHub interface](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/incorporating-changes-from-a-pull-request/reverting-a-pull-request).

To revert a pull request:
- Navigate to the merged pull request on GitHub.
- Click the Revert button. This action automatically creates a new branch and a
  pull request containing the revert commit.
- Edit the pull request title and description to comply with the
  [commit message guidelines](#commit-messages).
- The newly created revert pull request should be reviewed and merged following
  the same process as any other pull request.

Using the GitHub "Revert" button is the preferred method over manually creating
a revert commit using git revert.

### Keeping the pull request dashboard clean

We aim to keep the pull requests page clean so that we can quickly notice and
review incoming changes that require attention.  Given that goal, please do not
open a pull request unless you are ready for a code review. Draft pull requests
and ones without author activity for more than one business day may be closed
(they can always be reopened later).  If you're still working on something,
continue iterating on your branch without creating a pull request until it’s
ready for review.

### Addressing code review comments

Creating additional commits to address reviewer feedback is generally preferred
over amending and force-pushing. This makes it easier for reviewers to see what
has changed since their last review.  Pull requests are always squashed and
merged. Before merging, please review and edit the resulting commit message to
ensure it clearly describes the change.

After pushing,
[click the
button](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/proposing-changes-to-your-work-with-pull-requests/requesting-a-pull-request-review#requesting-reviews-from-collaborators-and-organization-members)
to ask a reviewer to re-request your review.

## Expectations for the team

A lot of our communication will happen on GitHub issues. Team members are
expected to configure their inboxes to receive GitHub notifications alerts for
all issues and pull requests to ensure effective communication.  If a pull
request becomes inactive or misaligned with current priorities, we may close it
to respect contributor and reviewer time. If you’d like to revisit it, just
comment and reopen the conversation.  If your pull request or issue is stuck,
feel free to follow up over chat. We encourage it!

### Reviewing a pull request

When reviewing a pull request:
- Start by reading the PR description to understand the purpose and context. If
  the commit message doesn’t follow the
  [commit message guidelines](#commit-messages),
  request changes.
- Use Approve or Request changes explicitly. Avoid leaving ambiguous feedback.
- Focus on what is in scope. If unrelated issues arise, suggest filing a
  separate PR or issue.
- If you’ve requested changes, approve the PR once the updates are
  satisfactory, even if the author forgot to click the re-request review.
- If a review has stalled or the context has shifted, leave a comment to
  clarify expectations, or close the PR. Keeping the dashboard clean is
  encouraged.

### Addressing Urgent Issues

We categorize issues into two primary levels of urgency:
- critical 🚨: requires immediate fix, should be treated as a p0 issue
- needs fix soon ❗: high priority issue, can be fixed during business hours

When an issue is labeled critical 🚨, the priority is to stabilize the system
enough to downgrade the severity to needs fix soon ❗.

### Maintaining a Healthy Main Branch

All pull requests require passing CI checks to be merged.

The main branch must always be stable, and tests should never fail at HEAD. A
red build on the main branch is a critical issue that must be fixed
immediately.  If tests become flaky or the main branch is not consistently
green, the team's top priority should shift to restoring stability. All feature
development should be deprioritized until green builds can be guaranteed.  When
you see a red x next to a commit on main, file an issue on your GitHub issue
tracker, and label it critical 🚨.  Create a PR to temporarily skip the test,
and verify that you have a green checkmark next to the commit on your main
branch. The issue can now be downgraded to needs fix soon ❗.

## Policy on new dependencies

While the Go ecosystem is rich with useful modules, in this project we try to
minimize the number of direct dependencies we have on modules that are not
Google-owned.

Adding new third party dependencies can have the following effects:
* broadens the vulnerability surface
* increases so called "vanity" import routing infrastructure failure points
* increases complexity of our own [`third_party`][] imports

So if you are contributing, please either contribute the full implementation
directly, or find a Google-owned project that provides the functionality. Of
course, there may be exceptions to this rule, but those should be well defined
and agreed upon by the maintainers ahead of time.

## Testing

We test code against two versions of Go, the minimum and maximum versions
supported by our clients. To see which versions these are checkout our
[README](README.md#supported-versions).

### Integration Tests

In addition to the unit tests, you may run the integration test suite. These
directions describe setting up your environment to run integration tests for
_all_ packages: note that many of these instructions may be redundant if you
intend only to run integration tests on a single package.

#### GCP Setup

To run the integrations tests, creation and configuration of three projects in
the Google Developers Console is required: one specifically for Firestore
integration tests, one specifically for Bigtable integration tests, and another
for all other integration tests. We'll refer to these projects as
"Firestore project", "Bigtable project" and "general project".

Note: You can skip setting up Bigtable project if you do not plan working on or running a few Bigtable
tests that require a secondary project

After creating each project, you must [create a service account](https://developers.google.com/identity/protocols/OAuth2ServiceAccount#creatinganaccount)
for each project. Ensure the project-level **Owner**
[IAM role](https://console.cloud.google.com/iam-admin/iam/project) role is added to
each service account. During the creation of the service account, you should
download the JSON credential file for use later.

Next, ensure the following APIs are enabled in the general project:

- BigQuery API
- BigQuery Data Transfer API
- Cloud Dataproc API
- Cloud Dataproc Control API Private
- Cloud Datastore API
- Cloud Firestore API
- Cloud Key Management Service (KMS) API
- Cloud Natural Language API
- Cloud OS Login API
- Cloud Pub/Sub API
- Cloud Resource Manager API
- Cloud Spanner API
- Cloud Speech API
- Cloud Translation API
- Cloud Video Intelligence API
- Cloud Vision API
- Compute Engine API
- Compute Engine Instance Group Manager API
- Container Registry API
- Firebase Rules API
- Google Cloud APIs
- Google Cloud Deployment Manager V2 API
- Google Cloud SQL
- Google Cloud Storage
- Google Cloud Storage JSON API
- Google Compute Engine Instance Group Updater API
- Google Compute Engine Instance Groups API
- Kubernetes Engine API
- Cloud Error Reporting API
- Pub/Sub Lite API

Next, create a Datastore database in the general project, and a Firestore
database in the Firestore project.

Finally, in the general project, create an API key for the translate API:

- Go to GCP Developer Console.
- Navigate to APIs & Services > Credentials.
- Click Create Credentials > API Key.
- Save this key for use in `GCLOUD_TESTS_API_KEY` as described below.

#### Local Setup

Once the three projects are created and configured, set the following
environment variables:

- `GCLOUD_TESTS_GOLANG_PROJECT_ID`: Developers Console project's ID (e.g.
bamboo-shift-455) for the general project.
- `GCLOUD_TESTS_GOLANG_KEY`: The path to the JSON key file of the general
project's service account.
- `GCLOUD_TESTS_GOLANG_DATASTORE_DATABASES`: Comma separated list of developer's
Datastore databases. If not provided, default database i.e. empty string is used.
- `GCLOUD_TESTS_GOLANG_FIRESTORE_PROJECT_ID`: Developers Console project's ID
(e.g. doorway-cliff-677) for the Firestore project.
- `GCLOUD_TESTS_GOLANG_FIRESTORE_DATABASES`: Comma separated list of developer's
Firestore databases. If not provided, default database is used.
- `GCLOUD_TESTS_GOLANG_FIRESTORE_KEY`: The path to the JSON key file of the
Firestore project's service account.
- `GCLOUD_TESTS_API_KEY`: API key for using the Translate API created above.
- `GCLOUD_TESTS_GOLANG_SECONDARY_BIGTABLE_PROJECT_ID`: Developers Console
project's ID (e.g. doorway-cliff-677) for Bigtable optional secondary project.
This can be same as Firestore project or any project other than the general
project.
- `GCLOUD_TESTS_BIGTABLE_CLUSTER`: Cluster ID of Bigtable cluster in general
project.
- `GCLOUD_TESTS_BIGTABLE_PRI_PROJ_SEC_CLUSTER`: Optional. Cluster ID of Bigtable
secondary cluster in general project
- `GCLOUD_TESTS_BIGTABLE_TAG_KEY`: The display name of the Resource Manager tag key.
- `GCLOUD_TESTS_BIGTABLE_TAG_VALUE`: The display name of the tag value.
- `TEST_UNIVERSE_DOMAIN`: Optional. Universe domain to test universe domain
functionality against.
- `TEST_UNIVERSE_PROJECT_ID`: Optional. Project ID within the universe domain
for testing.
- `TEST_UNIVERSE_LOCATION`: Optional. Available location within the universe
domain.
- `TEST_UNIVERSE_DOMAIN_CREDENTIAL`: Optional. The path to the JSON key file of
the universe domain's service account.

As part of the setup that follows, the following variables will be configured:

- `GCLOUD_TESTS_GOLANG_KEYRING`: The full name of the keyring for the tests,
in the form
"projects/P/locations/L/keyRings/R". The creation of this is described below.
- `GCLOUD_TESTS_BIGTABLE_KEYRING`: The full name of the keyring for the bigtable tests,
in the form
"projects/P/locations/L/keyRings/R". The creation of this is described below. Expected to be single region.
- `GCLOUD_TESTS_GOLANG_ZONE`: Compute Engine zone.

Install the [gcloud command-line tool][gcloudcli] to your machine and use it to
create some resources used in integration tests.

From the project's root directory:

``` sh
# Sets the default project in your env.
$ gcloud config set project $GCLOUD_TESTS_GOLANG_PROJECT_ID

# Authenticates the gcloud tool with your account.
$ gcloud auth login

# Create the indexes for all the databases you want to use in the datastore integration tests.
# Use empty string as databaseID or skip database flag for default database.
$ gcloud alpha datastore indexes create --database=your-databaseID-1 --project=$GCLOUD_TESTS_GOLANG_PROJECT_ID testdata/index.yaml

# Create the indexes for all the databases you want to use in the firestore integration tests.
# Use empty string as databaseID or skip database flag for default database.
# For TestIntegration_QueryDocuments_WhereEntity
$ gcloud firestore indexes composite create \
    --database=your-databaseID-1 --project=$GCLOUD_TESTS_GOLANG_FIRESTORE_PROJECT_ID \
    --collection-group=indexed_collection --query-scope=COLLECTION \
    --field-config=field-path=updatedAt,order=ascending \
    --field-config=field-path=weight,order=ascending \
    --field-config=field-path=height,order=ascending
$ gcloud firestore indexes composite create \
    --database=your-databaseID-1 --project=$GCLOUD_TESTS_GOLANG_FIRESTORE_PROJECT_ID \
    --collection-group=indexed_collection --query-scope=COLLECTION \
    --field-config=field-path=weight,order=ascending \
    --field-config=field-path=height,order=ascending

# For TestIntegration_QueryUnary
$ gcloud firestore indexes composite create \
    --database=your-databaseID-1 --project=$GCLOUD_TESTS_GOLANG_FIRESTORE_PROJECT_ID \
    --collection-group=indexed_collection --query-scope=COLLECTION \
    --field-config=field-path=testNull,order=ascending \
    --field-config=field-path=x,order=ascending \
    --field-config=field-path=q,order=ascending
$ gcloud firestore indexes composite create \
    --database=your-databaseID-1 --project=$GCLOUD_TESTS_GOLANG_FIRESTORE_PROJECT_ID \
    --collection-group=indexed_collection --query-scope=COLLECTION \
    --field-config=field-path=testNaN,order=ascending \
    --field-config=field-path=x,order=ascending \
    --field-config=field-path=q,order=ascending

# For TestIntegration_AggregationQueries
$ gcloud firestore indexes composite create \
    --database=your-databaseID-1 --project=$GCLOUD_TESTS_GOLANG_FIRESTORE_PROJECT_ID \
    --collection-group=indexed_collection --query-scope=COLLECTION \
    --field-config=field-path=weight,order=ascending \
    --field-config=field-path=model,order=ascending
$ gcloud firestore indexes composite create \
    --database=your-databaseID-1 --project=$GCLOUD_TESTS_GOLANG_FIRESTORE_PROJECT_ID \
    --collection-group=indexed_collection --query-scope=COLLECTION \
    --field-config=field-path=weight,order=ascending \
    --field-config=field-path=volume,order=ascending

# For TestIntegration_FindNearest (Vector Index)
$ gcloud alpha firestore indexes composite create \
    --database=your-databaseID-1 --project=$GCLOUD_TESTS_GOLANG_FIRESTORE_PROJECT_ID \
    --collection-group=indexed_collection --query-scope=COLLECTION \
    --field-config=field-path=EmbeddedField64,vector-config='{"dimension":"3","flat": "{}"}'

# Creates a Google Cloud storage bucket with the same name as your test project,
# and with the Cloud Logging service account as owner, for the sink
# integration tests in logging.
$ gcloud storage buckets create gs://$GCLOUD_TESTS_GOLANG_PROJECT_ID
$ gcloud storage buckets update "gs://$GCLOUD_TESTS_GOLANG_PROJECT_ID" \
--add-acl-grant=entity=group-cloud-logs@google.com,role=OWNER

# Creates a PubSub topic for integration tests of storage notifications.
$ gcloud beta pubsub topics create go-storage-notification-test
# Next, go to the Pub/Sub dashboard in GCP console. Authorize the user
# "service-<numeric project id>@gs-project-accounts.iam.gserviceaccount.com"
# as a publisher to that topic.

# Creates a Spanner instance for the spanner integration tests.
$ gcloud beta spanner instances create go-integration-test --config regional-us-central1 --nodes 10 --description 'Instance for go client test'
# NOTE: Spanner instances are priced by the node-hour, so you may want to
# delete the instance after testing with 'gcloud beta spanner instances delete'.

$ export MY_KEYRING=some-keyring-name
$ export MY_LOCATION=global
$ export MY_SINGLE_LOCATION=us-central1
# Creates a KMS keyring, in the same location as the default location for your
# project's buckets.
$ gcloud kms keyrings create $MY_KEYRING --location $MY_LOCATION
# Creates two keys in the keyring, named key1 and key2.
$ gcloud kms keys create key1 --keyring $MY_KEYRING --location $MY_LOCATION --purpose encryption
$ gcloud kms keys create key2 --keyring $MY_KEYRING --location $MY_LOCATION --purpose encryption
# Sets the GCLOUD_TESTS_GOLANG_KEYRING environment variable.
$ export GCLOUD_TESTS_GOLANG_KEYRING=projects/$GCLOUD_TESTS_GOLANG_PROJECT_ID/locations/$MY_LOCATION/keyRings/$MY_KEYRING
# Authorizes Google Cloud Storage to encrypt and decrypt using key1.
$ gcloud storage service-agent --project=$GCLOUD_TESTS_GOLANG_PROJECT_ID --authorize-cmek=$GCLOUD_TESTS_GOLANG_KEYRING/cryptoKeys/key1

# Create KMS Key in one region for Bigtable
$ gcloud kms keyrings create $MY_KEYRING --location $MY_SINGLE_LOCATION
$ gcloud kms keys create key1 --keyring $MY_KEYRING --location $MY_SINGLE_LOCATION --purpose encryption
# Sets the GCLOUD_TESTS_BIGTABLE_KEYRING environment variable.
$ export GCLOUD_TESTS_BIGTABLE_KEYRING=projects/$GCLOUD_TESTS_GOLANG_PROJECT_ID/locations/$MY_SINGLE_LOCATION/keyRings/$MY_KEYRING
# Create a service agent, https://cloud.google.com/bigtable/docs/use-cmek#gcloud:
$ gcloud beta services identity create \
    --service=bigtableadmin.googleapis.com \
    --project $GCLOUD_TESTS_GOLANG_PROJECT_ID
# Note the service agent email for the agent created.
$ export SERVICE_AGENT_EMAIL=<service agent email, from last step>

# Authorizes Google Cloud Bigtable to encrypt and decrypt using key1
$ gcloud kms keys add-iam-policy-binding key1 \
    --keyring $MY_KEYRING \
    --location $MY_SINGLE_LOCATION \
    --role roles/cloudkms.cryptoKeyEncrypterDecrypter \
    --member "serviceAccount:$SERVICE_AGENT_EMAIL" \
    --project $GCLOUD_TESTS_GOLANG_PROJECT_ID
```

It may be useful to add exports to your shell initialization for future use.
For instance, in `.zshrc`:

```sh
#### START GO SDK Test Variables
# Developers Console project's ID (e.g. bamboo-shift-455) for the general project.
export GCLOUD_TESTS_GOLANG_PROJECT_ID=your-project

# Developers Console project's ID (e.g. bamboo-shift-455) for the Bigtable project.
export GCLOUD_TESTS_GOLANG_SECONDARY_BIGTABLE_PROJECT_ID=your-bigtable-optional-secondary-project

# The path to the JSON key file of the general project's service account.
export GCLOUD_TESTS_GOLANG_KEY=~/directory/your-project-abcd1234.json

# Comma separated list of developer's Datastore databases. If not provided,
# default database i.e. empty string is used.
export GCLOUD_TESTS_GOLANG_DATASTORE_DATABASES=your-database-1,your-database-2

# Developers Console project's ID (e.g. doorway-cliff-677) for the Firestore project.
export GCLOUD_TESTS_GOLANG_FIRESTORE_PROJECT_ID=your-firestore-project

# Comma separated list of developer's Firestore databases. If not provided, default database is used.
export GCLOUD_TESTS_GOLANG_FIRESTORE_DATABASES=your-database-1,your-database-2

# The path to the JSON key file of the Firestore project's service account.
export GCLOUD_TESTS_GOLANG_FIRESTORE_KEY=~/directory/your-firestore-project-abcd1234.json

# The full name of the keyring for the tests, in the form "projects/P/locations/L/keyRings/R".
# The creation of this is described below.
export MY_KEYRING=my-golang-sdk-test
export MY_LOCATION=global
export GCLOUD_TESTS_GOLANG_KEYRING=projects/$GCLOUD_TESTS_GOLANG_PROJECT_ID/locations/$MY_LOCATION/keyRings/$MY_KEYRING

# API key for using the Translate API.
export GCLOUD_TESTS_API_KEY=abcdefghijk123456789

# Compute Engine zone. (https://cloud.google.com/compute/docs/regions-zones)
export GCLOUD_TESTS_GOLANG_ZONE=your-chosen-region
#### END GO SDK Test Variables
```

#### Running

Once you've done the necessary setup, you can run the integration tests by
running:

``` sh
$ go test -v ./...
```

Note that the above command will not run the tests in other modules. To run
tests on other modules, first navigate to the appropriate
subdirectory. For instance, to run only the tests for datastore:
``` sh
$ cd datastore
$ go test -v ./...
```

#### Replay

Some packages can record the RPCs during integration tests to a file for
subsequent replay. To record, pass the `-record` flag to `go test`. The
recording will be saved to the _package_`.replay` file. To replay integration
tests from a saved recording, the replay file must be present, the `-short`
flag must be passed to `go test`, and the `GCLOUD_TESTS_GOLANG_ENABLE_REPLAY`
environment variable must have a non-empty value.

## Contributor License Agreements

Before we can accept your pull requests you'll need to sign a Contributor
License Agreement (CLA):

- **If you are an individual writing original source code** and **you own the
intellectual property**, then you'll need to sign an [individual CLA][indvcla].
- **If you work for a company that wants to allow you to contribute your
work**, then you'll need to sign a [corporate CLA][corpcla].

You can sign these electronically (just scroll to the bottom). After that,
we'll be able to accept your pull requests.

## Contributor Code of Conduct

As contributors and maintainers of this project,
and in the interest of fostering an open and welcoming community,
we pledge to respect all people who contribute through reporting issues,
posting feature requests, updating documentation,
submitting pull requests or patches, and other activities.

We are committed to making participation in this project
a harassment-free experience for everyone,
regardless of level of experience, gender, gender identity and expression,
sexual orientation, disability, personal appearance,
body size, race, ethnicity, age, religion, or nationality.

Examples of unacceptable behavior by participants include:

* The use of sexualized language or imagery
* Personal attacks
* Trolling or insulting/derogatory comments
* Public or private harassment
* Publishing other's private information,
such as physical or electronic
addresses, without explicit permission
* Other unethical or unprofessional conduct.

Project maintainers have the right and responsibility to remove, edit, or reject
comments, commits, code, wiki edits, issues, and other contributions
that are not aligned to this Code of Conduct.
By adopting this Code of Conduct,
project maintainers commit themselves to fairly and consistently
applying these principles to every aspect of managing this project.
Project maintainers who do not follow or enforce the Code of Conduct
may be permanently removed from the project team.

This code of conduct applies both within project spaces and in public spaces
when an individual is representing the project or its community.

Instances of abusive, harassing, or otherwise unacceptable behavior
may be reported by opening an issue
or contacting one or more of the project maintainers.

This Code of Conduct is adapted from the [Contributor Covenant](https://contributor-covenant.org), version 1.2.0,
available at [https://contributor-covenant.org/version/1/2/0/](https://contributor-covenant.org/version/1/2/0/)

[gcloudcli]: https://developers.google.com/cloud/sdk/gcloud/
[indvcla]: https://developers.google.com/open-source/cla/individual
[corpcla]: https://developers.google.com/open-source/cla/corporate
[`third_party`]: https://opensource.google/documentation/reference/thirdparty
