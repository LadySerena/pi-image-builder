#!/usr/bin/env bash

set -x

device="/dev/sdc"

sudo umount ./mnt/boot/firmware

sudo umount ./mnt

sudo umount ./media-mnt/boot/firmware

sudo umount ./media-mnt

sudo losetup --detach-all

sudo wipefs -a /dev/rootvg/rootlv
sudo wipefs -a /dev/rootvg/csilv

sudo lvremove /dev/mapper/rootvg-rootlv
sudo lvremove /dev/mapper/rootvg-csilv

sudo vgremove rootvg

sudo pvremove "${device}2"

#sudo umount /run/media/serena/system-boot

sudo parted -s "${device}" rm 2

sudo parted -s "${device}" rm 1

sudo parted -s "${device}" print
