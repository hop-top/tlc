#!/bin/bash

# TLC Coverage Report Script
# Runs all tests and generates a combined coverage report.

set -e

# Create coverage directory
mkdir -p coverage

# Run tests with coverage
echo "Running tests with coverage..."
go test -v -coverprofile=coverage/all.out ./...

# Generate HTML report
echo "Generating HTML report..."
go tool cover -html=coverage/all.out -o coverage/all.html

# Display summary
echo "Coverage Summary:"
go tool cover -func=coverage/all.out

echo -e "\n✅ Coverage report generated at coverage/all.html"
