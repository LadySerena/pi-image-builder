#!/usr/bin/env bash

gcloud compute instances create image-build-arm --project=telvanni-platform --zone=us-central1-a \
--machine-type=t2a-standard-1 --network-interface=network-tier=PREMIUM,nic-type=GVNIC,subnet=default \
--maintenance-policy=MIGRATE --provisioning-model=STANDARD \
--service-account=tel-sa-pi-image-builder@telvanni-platform.iam.gserviceaccount.com \
--scopes=https://www.googleapis.com/auth/cloud-platform \
--create-disk=auto-delete=yes,boot=yes,device-name=image-build-arm,image=projects/debian-cloud/global/images/debian-11-bullseye-arm64-v20220920,mode=rw,size=20,type=projects/telvanni-platform/zones/us-central1-a/diskTypes/pd-balanced \
--no-shielded-secure-boot --shielded-vtpm --shielded-integrity-monitoring --reservation-affinity=any