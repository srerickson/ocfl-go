FROM mcr.microsoft.com/devcontainers/go:dev-1.25
RUN echo 'deb [trusted=yes] https://repo.goreleaser.com/apt/ /' | sudo tee /etc/apt/sources.list.d/goreleaser.list
RUN apt-get update && apt-get upgrade -y && apt-get install -y goreleaser