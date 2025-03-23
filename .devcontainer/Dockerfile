FROM mcr.microsoft.com/devcontainers/go:1.4.0-1.24-bookworm
RUN echo 'deb [trusted=yes] https://repo.goreleaser.com/apt/ /' | sudo tee /etc/apt/sources.list.d/goreleaser.list
RUN apt-get update && apt-get upgrade -y && apt-get install -y goreleaser