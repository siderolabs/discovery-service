# Talos Discovery Service (for KubeSpan)

Discovery Service provides cluster membership and KubeSpan peer information for Talos clusters.

## Overview

Discovery Service provides centralized service for Talos nodes to exchange information about nodes of the cluster.

Talos runs "official" instance of the service, and anyone can run their own instance on-prem or in the cloud.

Discovery service doesn't store any data, all data is ephemeral and is kept only in memory.
Node information is expired (if not updated) after 30 minutes.
Discovery service doesn't see actual node information, it only stores and updates encrypted blobs.
Discovery data should be submitted encrypted by the client, and service doesn't have the encryption key.

The project has been split into 3 parts (as they have different source code licenses), namely:

- This repository
- [Discovery Client](https://github.com/siderolabs/discovery-client), contains the client code to interact with the server
- [Discovery Service API](https://github.com/talos-systems/discovery-api/), provides gRPC API definition and cluster data protobuf data structures

## Setup

All of the details to get started are present [in the Makefile](Makefile), start with `make help`.

## Interacting

Once the application is running you can test the grpc functionality on the port 3000 and the http pages in browser on port 3001.

To test the grpc calls, [install grpcurl](https://github.com/fullstorydev/grpcurl#installation) and clone the [Discovery Service API](https://github.com/talos-systems/discovery-api/) repository.

- Sample code-block to test the grpc `Hello` call (change path accordingly)

    ``` bash
    grpcurl -proto v1alpha1/server/cluster.proto -import-path PATH_TO_REPO/discovery-api/api -plaintext -d '{"clusterId": "abc"}' -H 'X-Real-IP: 1.2.3.4' localhost:3000 sidero.discovery.server.Cluster/Hello
    ```
