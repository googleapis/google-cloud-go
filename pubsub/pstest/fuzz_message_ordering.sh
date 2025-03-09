#!/bin/bash

# Default values for flags
print_output=false
count_tests=true
iterations=100

# Parse command-line flags
while getopts "pc:i:" opt; do
  case $opt in
    p) print_output=true ;;  # Enable printing output
    c) count_tests=true ;;   # Enable counting
    i) iterations=$OPTARG ;; # Set the number of iterations (default 1000)
    \?) echo "Usage: $0 [-p] [-c] [-i iterations]"
        exit 1 ;;
  esac
done

# Variables to count passes and fails
pass_count=0
fail_count=0

# Loop through test runs
for (( i=1; i<=iterations; i++ )); do
    output=$(go test -timeout 2s -run ^TestSubscriptionMessageOrdering$ cloud.google.com/go/pubsub/pstest -count=1)

    # Print the output if the flag is set
    if $print_output; then
        echo "$output"
    fi

    # Count pass/fail if the flag is set
    if $count_tests; then
        if echo "$output" | grep -q "ok\s*cloud.google.com/go/pubsub/pstest"; then
            ((pass_count++))
        elif echo "$output" | grep -q "FAIL\s*cloud.google.com/go/pubsub/pstest"; then
            ((fail_count++))
        fi
    fi

    # Print progress after each iteration
    if $count_tests; then
        echo "Iteration $i/$iterations - Passed: $pass_count, Failed: $fail_count"
    fi
done

# Print final count results if the flag is set
if $count_tests; then
    echo "Final Results: Passed tests: $pass_count, Failed tests: $fail_count"
fi
