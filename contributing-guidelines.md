# Contributing to Kaiwo

We welcome contributions to Kaiwo! Please read the following guidelines before contributing.

## How to contribute

At the moment, we have relatively light requirements for contributors. We are looking for people who are interested in contributing to the project in any way. This could be through code contributions, documentation, or even just providing feedback on the project.

The easiest way to contribute is to open an issue on the GitHub repository. This could be a bug report, a feature request, or even just a question. We will do our best to respond to all issues in a timely manner.

If you are interested in contributing code to the project, please follow the guidelines below.

### Guidelines for code contributions

1. Fork the repository on GitHub.
2. Clone your fork of the repository to your local machine.
3. Create a new branch for your changes.
4. Make your changes on the new branch.
5. Make sure your changes pass all linters and tests.
6. Above all, make sure that example workloads are still running to `completed` status after your changes.
7. Push your changes to your fork on GitHub.
8. Create a pull request from your fork to the main repository.
9. We will assign a reviewer to your pull request, who will review your changes and provide feedback.

## Setting Up Your Development Environment

To set up your development environment, follow these steps:

1. Clone the repository to your local machine.
2. Install Go on your machine.
3. Install the dependencies by running `go mod tidy`.
4. Install GolangCI-Lint by running `brew install golangci-lint`
5. Install pre-commit hooks by running `pre-commit install`
