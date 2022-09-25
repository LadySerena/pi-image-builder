#!/usr/bin/env bash

set -x

sudo umount ./mnt/boot/firmware

sudo umount ./mnt

sudo umount ./media-mnt/boot/firmware

sudo umount ./media-mnt

sudo losetup --detach-all

sudo wipefs -a /dev/rootvg/rootlv

sudo lvremove /dev/mapper/rootvg-rootlv

sudo vgremove rootvg

sudo pvremove /dev/sdb2

#sudo umount /run/media/serena/system-boot

sudo parted -s /dev/sdb rm 2

sudo parted -s /dev/sdb rm 1

sudo parted -s /dev/sdb print
