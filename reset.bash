#!/usr/bin/env bash

set -x

# leaving this hardcoded because there are 0 safety checks
device="/dev/sdb"

sudo umount ./mnt/boot/firmware

sudo umount ./mnt

sudo umount ./media-mnt/boot/firmware

sudo umount ./media-mnt

sudo losetup --detach-all

sudo wipefs -a /dev/rootvg/rootlv
sudo wipefs -a /dev/rootvg/csilv
sudo wipefs -a /dev/rootvg/containerdlv

sudo lvremove /dev/mapper/rootvg-rootlv
sudo lvremove /dev/mapper/rootvg-csilv
sudo lvremove /dev/mapper/rootvg-containerdlv

sudo vgremove rootvg

sudo pvremove "${device}2"

#sudo umount /run/media/serena/system-boot

sudo parted -s "${device}" rm 2

sudo parted -s "${device}" rm 1

sudo parted -s "${device}" print

sudo dmsetup remove /dev/mapper/rootvg-rootlv
sudo dmsetup remove /dev/mapper/rootvg-csilv
sudo dmsetup remove /dev/mapper/rootvg-containerdlv
