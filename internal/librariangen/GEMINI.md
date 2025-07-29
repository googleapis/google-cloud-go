# Go development workflow rules

After any code change, do the following steps:

1. Run `go build .` If the build passes, go to step 2. If the build has any errors, repeat this step. If you can't fix the build in 3 tries, ask for help.
2. Run `go test ./...`. If the tests pass, go to step 3. If you can't fix the build in 3 tries, ask for help.
3. After the existing unit tests pass, ask to run the binary integration test script with this command: `run source ~/.zshrc && ./run-binary-integration-test.sh`
4. Check `librariangen.log` to verify the output. Analyze the output and ask if anything needs to be fixed.
5. If the integration test passes and the output is correct, then devise a plan to add or update tests to ensure the new logic is covered. Show the plan. Do not make any code changes until the plan is approved. This is a step where you often get confused, so read the latest relevant files and think hard.
6. Once your plan to update unit tests is approved, make the code change to the tests. If you can't update the tests in 3 tries, ask for help.
7. Run `go test ./...`. If the tests pass, go to step 8. If you can't fix the build in 3 tries, ask for help.
8. Offer to commit the changes with `git commit`.
   
