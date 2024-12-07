set -ex
git checkout main ../storage_control_client.go
go run applyStorageV2Rpcs.go > ../storage_control_client_mod.go
mv ../storage_control_client_mod.go ../storage_control_client.go