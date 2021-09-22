# Talos Discovery Service (for KubeSpan)

Discovery Service provides cluster membership and KubeSpan peer information for Talos clusters.

## Overview

Discovery Service provides centralized service for Talos nodes to exchange information about nodes of the cluster.

Talos runs "official" instance of the service, and anyone can run their own instance on-prem or in the cloud.

Discovery service doesn't store any data, all data is ephemeral and is kept only in memory.
Node information is expired (if not updated) after 30 minutes.
Discovery service doesn't see actual node information, it only stores and updates encrypted blobs.
Discovery data should be submitted encrypted by the client, and service doesn't have the encryption key.
