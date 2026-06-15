---
title: "Installation"
description: "Install biostudies from a release, with go install, or from source."
weight: 20
---

## Prebuilt binaries

Every [release](https://github.com/tamnd/biostudies-cli/releases) carries archives for Linux, macOS,
and Windows on amd64 and arm64, plus deb, rpm, and apk packages for Linux.
Download, unpack, put `biostudies` on your `PATH`, done. The `checksums.txt`
on each release is signed with keyless [cosign](https://docs.sigstore.dev/) if
you want to verify before running.

## With Go

```bash
go install github.com/tamnd/biostudies-cli/cmd/biostudies@latest
```

That puts `biostudies` in `$(go env GOPATH)/bin`, which is `~/go/bin` unless
you moved it. Make sure that directory is on your `PATH`.

## From source

```bash
git clone https://github.com/tamnd/biostudies-cli
cd biostudies-cli
make build        # produces ./bin/biostudies
./bin/biostudies version
```

## Container image

```bash
docker run --rm ghcr.io/tamnd/biostudies:latest --help
```

## Checking the install

```bash
biostudies version
```

prints the version and exits.
