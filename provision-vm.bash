#!/usr/bin/env bash

go_tar_ball="go1.19.2.linux-arm64.tar.gz"

sudo apt-get update -y

sudo apt-get install systemd-container e2fsprogs dosfstools lvm2 rsync xz-utils git -y

curl -LO https://go.dev/dl/${go_tar_ball}

sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf "$go_tar_ball"

# shellcheck disable=SC2016
echo 'export PATH=$PATH:/usr/local/go/bin' | sudo tee -a  /etc/profile