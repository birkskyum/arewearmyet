#!/usr/bin/env bash
# Copies what probe.sh reads from this machine into a folder, as a fixture for test/probe.test.ts:
# small files from /sys, /proc and /etc, and the output of the few commands the probe runs.
# No serial numbers or addresses are among them. Usage: bash scripts/probe-fixture.sh <folder>
set -u
out="${1:?usage: probe-fixture.sh <folder>}"
mkdir -p "$out"

copy() { # copy <file>...
  local f
  for f in "$@"; do
    [ -f "$f" ] && [ -r "$f" ] || continue
    mkdir -p "$out$(dirname "$f")"
    head -c 4000 "$f" >"$out$f" 2>/dev/null
  done
}
dir() { local d; for d in "$@"; do [ -e "$d" ] && mkdir -p "$out$d"; done; }
# A device's driver link, kept as a link to a folder with the driver's name
driver() { # driver <path of the "driver" link>
  [ -e "$1" ] || return
  local name; name="$(basename "$(readlink -f "$1")")"
  mkdir -p "$out/sys/_drivers/$name" "$out$(dirname "$1")"
  ln -sfn "../../../../_drivers/$name" "$out$1"
}

copy /sys/class/dmi/id/{sys_vendor,product_name,product_family,product_sku,board_name,bios_version,chassis_type}
copy /proc/device-tree/compatible /proc/asound/cards /etc/os-release
grep -s '^LIBGL_ALWAYS_SOFTWARE' /etc/environment >/dev/null && { mkdir -p "$out/etc"; grep '^LIBGL_ALWAYS_SOFTWARE' /etc/environment >"$out/etc/environment"; }
copy /sys/class/drm/card*-*/{status,modes}
for r in /sys/class/drm/renderD*; do driver "$r/device/driver"; done
dir /sys/bus/platform/drivers/adreno /sys/bus/platform/drivers/adreno/*gpu*
copy /sys/class/backlight/*/max_brightness
copy /sys/class/input/event*/device/name /sys/class/input/event*/device/capabilities/sw
mkdir -p "$out/udev"
for ev in /sys/class/input/event*; do udevadm info -q property -p "$ev" 2>/dev/null | grep -E '^ID_(BUS|INPUT)' >"$out/udev/${ev##*/}"; done
for n in /sys/class/net/*; do
  [ -d "$n/wireless" ] || [ -d "$n/phy80211" ] || continue
  dir "$n/wireless"; copy "$n/operstate" "$n/addr_assign_type"; driver "$n/device/driver"
done
dir /sys/class/bluetooth/hci*
copy /sys/class/rfkill/rfkill*/{type,soft,hard}
copy /sys/class/remoteproc/remoteproc*/{name,state,firmware}
copy /sys/class/power_supply/*/{type,capacity,status,energy_now,energy_full,charge_now,charge_full,online,usb_type,voltage_now,current_now}
copy /sys/power/{mem_sleep,state} /sys/power/suspend_stats/{success,fail}
copy /sys/bus/usb/devices/usb*/speed /sys/bus/usb/devices/*/{idVendor,idProduct}
copy /sys/bus/thunderbolt/devices/*/{device_name,rx_speed,rx_lanes}
dir /sys/bus/thunderbolt/devices/*
copy /sys/class/video4linux/video*/name /sys/class/video4linux/v4l-subdev*/name
copy /sys/class/hwmon/hwmon*/name /sys/class/hwmon/hwmon*/fan*_input /sys/class/thermal/thermal_zone*/temp

mkdir -p "$out/cmd"
uname -r >"$out/cmd/uname-r"
uname -m >"$out/cmd/uname-m"
command -v systemd-analyze >/dev/null 2>&1 && systemd-analyze chid >"$out/cmd/chids" 2>/dev/null
{ journalctl -k -b --no-pager -q -o cat 2>/dev/null || dmesg 2>/dev/null; } | grep -iE 'zap|no GPU device was found' | head -n 20 >"$out/cmd/klog"
command -v bluetoothctl >/dev/null 2>&1 && timeout 5 bluetoothctl show 2>/dev/null | grep -E '^\s*Powered:' >"$out/cmd/bluetoothctl-show"
for p in linux-omarchy linux-firmware linux-firmware-qcom mesa systemd omarchy hyprland pipewire alsa-ucm-conf bluez libcamera; do
  command -v pacman >/dev/null 2>&1 && pacman -Q "$p" 2>/dev/null
done >"$out/cmd/packages"
# Git doesn't keep empty folders, and some of these only matter by being there
find "$out" -type d -empty -exec touch {}/.keep \;
echo "Fixture written to $out"
