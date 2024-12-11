set -ex
# git clone https://github.com/googleapis/googleapis.git ../../../../.googleapis
git checkout main ../storage_control_client.go
pushd ../../../../
# git -C .googleapis clean -f -d -x
# git -C .googleapis restore .
# git -C .googleapis pull
# env -C .googleapis/ patch -p1 < storage/control/apiv2/internal/merge_storage_v2.patch
env -C .googleapis/ bazelisk fetch //google/storage/control/v2:control_go_proto //google/storage/control/v2:control_go_gapic //google/storage/control/v2:gapi-cloud-storage-control-v2-go
env -C .googleapis/ bazelisk build //google/storage/control/v2:control_go_proto //google/storage/control/v2:control_go_gapic //google/storage/control/v2:gapi-cloud-storage-control-v2-go
bazel_output=$(env -C .googleapis/ bazelisk info output_path)
tar -zx --strip-components=3 -f ${bazel_output}/k8-fastbuild/bin/google/storage/control/v2/gapi-cloud-storage-control-v2-go.tar.gz ./cloud.google.com/go/storage/control/apiv2/
popd
go run postprocessor.go > ../storage_control_client_mod.go
mv ../storage_control_client_mod.go ../storage_control_client.go