#!/usr/bin/env bash
# Are we ARM yet? probe: checks what works on a Snapdragon PC under Linux and,
# with your OK, sends the result to arewearmyet.com for this model's page.
#
#   curl -fsSL https://arewearmyet.com/probe.sh | bash                 quick check, read-only, about 5 seconds
#   curl -fsSL https://arewearmyet.com/probe.sh | bash -s -- --guided  also tests sound, camera and sleep with you
#
# Options:
#   --guided      after the quick check, test speakers, microphones, camera and sleep, and ask about the rest
#   --json        print what would be sent and stop (for agents and the curious)
#   --send        send without asking
#   --no-send     only show the results
#   --model ID    the model id from arewearmyet.com, when the machine isn't recognised by itself
#   --agent NAME  the agent sending this, shown next to the run
#   --api URL     another server, for testing
#
# The quick check only reads files under /sys, /proc and /etc and runs read-only
# commands; it needs no root and changes nothing. The guided test plays two short
# tones, records a few seconds from the microphones, takes one camera frame and
# waits for you to close and open the lid; recordings stay on the machine and are
# deleted at the end. The results are shown, then sent unless you say no (in a
# terminal) or pass --no-send (without one, as agents run it). What's sent: the maker,
# model, SKU, board and BIOS version; the kernel, OS and a few package versions;
# the results; its hardware IDs, which are made from those same strings. No serial
# numbers, MAC addresses, user names, host names or files.
# A random token in ~/.local/state/arewearmyet/token lets this machine's later
# runs replace its earlier ones on the site.

set -u
LC_ALL=C
export LC_ALL

API="https://arewearmyet.com"
GUIDED=0
JSON_ONLY=0
SEND=ask
MODEL=""
AGENT=""
while [ $# -gt 0 ]; do
  case "$1" in
    --guided) GUIDED=1 ;;
    --json) JSON_ONLY=1 ;;
    --send) SEND=yes ;;
    --no-send) SEND=no ;;
    --model) MODEL="${2:-}"; shift ;;
    --agent) AGENT="${2:-}"; shift ;;
    --api) API="${2:-}"; shift ;;
    -h|--help) sed -n '2,28p' "$0" 2>/dev/null || echo "See https://arewearmyet.com/agents"; exit 0 ;;
    *) echo "Unknown option $1" >&2; exit 2 ;;
  esac
  shift
done

# For the tests: a folder holding a copy of another machine's /sys, /proc and /etc
R="${PROBE_ROOT:-}"

TMP="$(mktemp -d "${TMPDIR:-/tmp}/arewearmyet.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT

# Output for people goes to stderr, so --json leaves clean JSON on stdout
say() { printf '%s\n' "$*" >&2; }
TTY=""
if [ -r /dev/tty ] && [ -w /dev/tty ] && { : </dev/tty; } 2>/dev/null; then TTY=/dev/tty; fi
ask() { # ask "question" -> REPLY
  REPLY=""
  [ -n "$TTY" ] || return 1
  # Keys pressed while a step was still running mustn't answer the next question
  while IFS= read -r -t 0.05 -n 1000 _ <"$TTY"; do :; done
  printf '%s ' "$1" >"$TTY"
  IFS= read -r REPLY <"$TTY" || return 1
}

read1() { [ -r "$1" ] && head -c 4000 "$1" 2>/dev/null | tr -d '\0' | head -n 1; }
json_str() { # escape for a JSON string
  local s="$1"
  s="${s//\\/\\\\}"; s="${s//\"/\\\"}"; s="${s//$'\n'/ }"; s="${s//$'\r'/ }"; s="${s//$'\t'/ }"
  printf '"%s"' "$s"
}
# Addresses and home folders never leave the machine, even inside evidence
scrub() { sed -E 's/([0-9A-Fa-f]{2}[:-]){5}[0-9A-Fa-f]{2}/xx:xx:xx:xx:xx:xx/g; s#/home/[^/ ]+#/home/user#g'; }

RESULTS=()     # JSON objects
declare -A STATE_OF
add() { # add check state how evidence [note]
  local check="$1" state="$2" how="$3" evidence="${4:-}" note="${5:-}"
  evidence="$(printf '%s' "$evidence" | scrub | head -c 380)"
  note="$(printf '%s' "$note" | scrub | head -c 580)"
  # A later result for the same check (the guided test after the quick one) replaces the earlier
  local i kept=()
  for i in "${RESULTS[@]+"${RESULTS[@]}"}"; do
    case "$i" in *"\"check\":\"$check\""*) ;; *) kept+=("$i") ;; esac
  done
  RESULTS=("${kept[@]+"${kept[@]}"}")
  local obj="{\"check\":\"$check\",\"state\":\"$state\",\"how\":\"$how\""
  [ -n "$evidence" ] && obj+=",\"evidence\":$(json_str "$evidence")"
  [ -n "$note" ] && obj+=",\"note\":$(json_str "$note")"
  RESULTS+=("$obj}")
  STATE_OF[$check]="$state"
}

# --- The machine ---------------------------------------------------------------
DMI=$R/sys/class/dmi/id
MAKER="$(read1 $DMI/sys_vendor)"; PRODUCT="$(read1 $DMI/product_name)"; FAMILY="$(read1 $DMI/product_family)"
SKU="$(read1 $DMI/product_sku)"; BOARD="$(read1 $DMI/board_name)"; BIOS="$(read1 $DMI/bios_version)"
CHASSIS="$(read1 $DMI/chassis_type)"
COMPATIBLE=""
[ -r $R/proc/device-tree/compatible ] && COMPATIBLE="$(tr '\0' '\n' <$R/proc/device-tree/compatible | head -n 1)"
KERNEL="$(uname -r)"
OS="$( . "$R/etc/os-release" 2>/dev/null; printf '%s' "${PRETTY_NAME:-${NAME:-Linux}}")"
case "$(uname -m)" in aarch64|arm64) ;; *) say "This is a $(uname -m) machine; the probe is for ARM PCs. Stopping."; exit 1 ;; esac
LAPTOP=1
case "$CHASSIS" in 3|4|5|6|7|13|15|16|35|36) LAPTOP=0 ;; esac  # desktops, mini PCs, all-in-ones

PKGS=""
pkgver() { # pkgver name -> version or empty
  if command -v pacman >/dev/null 2>&1; then pacman -Q "$1" 2>/dev/null | awk '{print $2}'
  elif command -v dpkg-query >/dev/null 2>&1; then dpkg-query -W -f '${Version}' "$1" 2>/dev/null
  elif command -v rpm >/dev/null 2>&1; then rpm -q --qf '%{VERSION}-%{RELEASE}' "$1" 2>/dev/null | grep -v 'not installed'
  fi
}
for p in linux-omarchy linux-firmware linux-firmware-qcom mesa systemd omarchy hyprland pipewire alsa-ucm-conf bluez libcamera; do
  v="$(pkgver "$p")"
  [ -n "$v" ] && PKGS+="${PKGS:+,}$(json_str "$p"):$(json_str "$v")"
done

# The machine's hardware IDs, most specific first, which name its model exactly (systemd 258 and later)
CHIDS=""
if command -v systemd-analyze >/dev/null 2>&1; then
  while read -r t c; do CHIDS+="${CHIDS:+,}\"$t:$c\""; done < <(timeout -k 2 10 systemd-analyze chid 2>/dev/null | awk '$3 ~ /^[0-9a-f]+-[0-9a-f]+-/ { print $1, $3 }')
fi

say "Are we ARM yet? probe"
say "  ${MAKER:-?} ${PRODUCT:-?} ${FAMILY:+($FAMILY)}${COMPATIBLE:+, device tree $COMPATIBLE}"
say "  kernel $KERNEL on $OS"
say ""

# --- Quick check: only what can be measured -----------------------------------
# Kernel log, when the user may read it (wheel and adm usually can)
KLOG="$TMP/klog"
journalctl -k -b --no-pager -q -o cat >"$KLOG" 2>/dev/null || dmesg >"$KLOG" 2>/dev/null || : >"$KLOG"

# Built-in screen
for c in $R/sys/class/drm/card*-eDP-* $R/sys/class/drm/card*-DSI-*; do
  [ -e "$c/status" ] || continue
  st="$(read1 "$c/status")"; mode="$(read1 "$c/modes")"; name="${c##*/}"
  if [ "$st" = connected ]; then add display works measured "${name#card?-} connected, ${mode:-no mode listed}"
  else add display broken measured "${name#card?-} $st"; fi
  break
done

# GPU: the adreno driver bound to a GPU, or the msm display driver on its own
gpu_dev=""
for d in $R/sys/bus/platform/drivers/adreno/*; do case "${d##*/}" in *gpu*) gpu_dev="${d##*/}" ;; esac; done
render_drv=""
for r in $R/sys/class/drm/renderD*; do [ -e "$r/device/driver" ] && render_drv="$(basename "$(readlink -f "$r/device/driver")")"; done
soft=""
grep -qs '^LIBGL_ALWAYS_SOFTWARE=1' "$R/etc/environment" && soft=1
if [ -n "$gpu_dev" ]; then
  zap="$(grep -i 'zap' "$KLOG" | grep -iE 'fail|error' | head -n 1)"
  if [ -n "$zap" ]; then add gpu broken measured "GPU $gpu_dev bound, but: $zap"
  elif [ -n "$soft" ]; then add gpu partial measured "GPU $gpu_dev bound, but /etc/environment forces software rendering"
  else add gpu works measured "adreno bound to $gpu_dev, render node from ${render_drv:-?}"; fi
elif [ "$render_drv" = msm ] || grep -q 'no GPU device was found' "$KLOG"; then
  add gpu broken measured "msm drives the display, but no GPU is bound (the device tree has it off or its firmware failed)${soft:+; software rendering is on}"
fi

# Backlight: without a device the brightness keys can't do anything
bl=""
for b in $R/sys/class/backlight/*; do [ -e "$b/max_brightness" ] && bl="${b##*/} max $(read1 "$b/max_brightness")"; done
[ -z "$bl" ] && [ "$LAPTOP" = 1 ] && add backlight broken measured "no backlight device"

# Input devices, by udev's view of them
KB=""; TP=""; TS=""; LID=""
for ev in $R/sys/class/input/event*; do
  [ -e "$ev/device/name" ] || continue
  name="$(read1 "$ev/device/name")"
  props="$(udevadm info -q property -p "$ev" 2>/dev/null)"
  bus="$(printf '%s\n' "$props" | sed -n 's/^ID_BUS=//p')"
  case "$bus" in usb|bluetooth) continue ;; esac
  case "$name" in *[Gg][Pp][Ii][Oo]*|*[Pp]ower*|*pmic*|*PMIC*|*[Vv]ideo\ [Bb]us*|*[Hh]eadset*|*[Jj]ack*) ;;
    *) printf '%s\n' "$props" | grep -q '^ID_INPUT_KEYBOARD=1' && KB="$name" ;;
  esac
  printf '%s\n' "$props" | grep -q '^ID_INPUT_TOUCHPAD=1' && TP="$name"
  printf '%s\n' "$props" | grep -q '^ID_INPUT_TOUCHSCREEN=1' && TS="$name"
  sw="$(read1 "$ev/device/capabilities/sw")"
  if [ -n "$sw" ] && [ "$sw" != 0 ] && [ $(( 0x${sw##* } & 1 )) -eq 1 ]; then LID="$name"; fi
done
if [ "$LAPTOP" = 1 ]; then
  [ -z "$KB" ] && add keyboard broken measured "no built-in keyboard device"
  [ -z "$TP" ] && add touchpad broken measured "no built-in touchpad device"
  [ -z "$LID" ] && add lid broken measured "no input device reports a lid switch"
fi

# Wi-Fi
wif=""
for n in $R/sys/class/net/*; do [ -d "$n/wireless" ] || [ -d "$n/phy80211" ] && wif="${n##*/}"; done
if [ -z "$wif" ]; then
  add wifi broken measured "no Wi-Fi interface"
else
  drv="$(basename "$(readlink -f "$R/sys/class/net/$wif/device/driver" 2>/dev/null)" 2>/dev/null)"
  oper="$(read1 "$R/sys/class/net/$wif/operstate")"
  [ "$oper" = up ] && add wifi works measured "$wif ($drv) connected"
  at="$(read1 "$R/sys/class/net/$wif/addr_assign_type")"
  case "$at" in
    0) add wifi-address works measured "$wif ($drv): permanent address" ;;
    1) add wifi-address broken measured "$wif ($drv): random address made by the kernel, so it changes every boot" ;;
    3) add wifi-address works measured "$wif ($drv): address set by the system (not from the card)" ;;
  esac
fi

# Bluetooth
bt=""
for h in $R/sys/class/bluetooth/hci*; do [ -e "$h" ] && bt="${h##*/}"; done
if [ -z "$bt" ]; then add bluetooth broken measured "no Bluetooth controller"
elif command -v bluetoothctl >/dev/null 2>&1; then
  pw="$(timeout -k 2 5 bluetoothctl show 2>/dev/null | sed -n 's/^[[:space:]]*Powered: //p' | head -n 1)"
  [ "$pw" = yes ] && add bluetooth works measured "$bt powered"
  # Switched off by the owner, as in airplane mode, says nothing about the hardware
  bt_off=""
  for rf in $R/sys/class/rfkill/rfkill*; do [ "$(read1 "$rf/type")" = bluetooth ] && [ "$(read1 "$rf/soft")" = 1 ] && bt_off=1; done
  [ "$pw" = no ] && [ -z "$bt_off" ] && add bluetooth partial measured "$bt present, but it doesn't power on"
fi

# Remote processors: the audio DSP (sound, battery, USB-C power) and the compute DSP
for rp in $R/sys/class/remoteproc/remoteproc*; do
  [ -e "$rp/name" ] || continue
  name="$(read1 "$rp/name")"; st="$(read1 "$rp/state")"; fw="$(read1 "$rp/firmware")"
  check=""
  case "$name" in *adsp*) check=adsp ;; *cdsp*) check=cdsp ;; esac
  [ -n "$check" ] || continue
  case "$st" in
    running) add "$check" works measured "$name running ${fw:+($fw)}" ;;
    crashed) add "$check" broken measured "$name crashed ${fw:+($fw)}" ;;
    offline) add "$check" broken measured "$name offline ${fw:+(wants $fw)}" ;;
  esac
done

# Sound cards on the machine itself (not HDMI or USB ones)
card=""
while read -r line; do
  case "$line" in *USB-Audio*|*HDMI*|*hdmi*) ;; *\]:*) card="$line" ;; esac
done < <(grep ']:' $R/proc/asound/cards 2>/dev/null)
if [ -z "$card" ]; then
  add speakers broken measured "no built-in sound card"
  add microphones broken measured "no built-in sound card"
fi

# Battery and charging
bat=""; charge=""
for ps in $R/sys/class/power_supply/*; do
  t="$(read1 "$ps/type")"
  case "$t" in
    Battery)
      cap="$(read1 "$ps/capacity")"; st="$(read1 "$ps/status")"
      # Qualcomm's battery manager gives energy or charge, not always a percentage
      if [ -z "$cap" ]; then
        for pair in energy charge; do
          now="$(read1 "$ps/${pair}_now")"; full="$(read1 "$ps/${pair}_full")"
          if [ -n "$now" ] && [ -n "$full" ] && [ "$full" -gt 0 ] 2>/dev/null; then cap=$(( now * 100 / full )); break; fi
        done
      fi
      [ -n "$cap" ] && bat="${ps##*/} at $cap%, $st" ;;
    USB|Mains|USB_PD|USB_C)
      [ "$(read1 "$ps/online")" = 1 ] || continue
      kind="$(read1 "$ps/usb_type" | sed -nE 's/.*\[([^]]*)\].*/\1/p')"
      v="$(read1 "$ps/voltage_now")"; a="$(read1 "$ps/current_now")"
      desc="${ps##*/}${kind:+ $kind}"
      [ "${v:-0}" -gt 0 ] 2>/dev/null && desc+=", $((v / 1000)) mV"
      [ "${a:-0}" -gt 0 ] 2>/dev/null && desc+=", $((a / 1000)) mA"
      case "$kind" in
        PD|PD_PPS|PD_DRP) charge="works|$desc" ;;
        *) if [ "${v:-0}" -gt 6000000 ] 2>/dev/null; then charge="works|$desc"
           elif [ -z "$charge" ] && [ "${v:-0}" -gt 0 ] 2>/dev/null; then charge="partial|$desc (5 V only)"; fi ;;
      esac ;;
  esac
done
if [ -n "$bat" ]; then add battery works measured "$bat"
elif [ "$LAPTOP" = 1 ]; then add battery broken measured "no battery reports its charge"; fi
[ -n "$charge" ] && add charging "${charge%%|*}" measured "${charge#*|}"

# Sleep since boot: a machine that slept and is running now woke up from it
ms="$(read1 $R/sys/power/mem_sleep)"; ok="$(read1 $R/sys/power/suspend_stats/success)"; bad="$(read1 $R/sys/power/suspend_stats/fail)"
if [ -n "$ok" ] && [ "$ok" -gt 0 ] 2>/dev/null; then
  if [ "${bad:-0}" -gt 0 ]; then add suspend partial measured "$ok sleeps and $bad failures since boot, modes $ms"
  else add suspend works measured "$ok sleeps since boot, none failed, modes $ms"; fi
elif [ -n "$bad" ] && [ "$bad" -gt 0 ] 2>/dev/null; then add suspend broken measured "$bad failed sleeps since boot, modes $ms"; fi
grep -qw disk $R/sys/power/state 2>/dev/null || add hibernate broken measured "the kernel offers no hibernation"

# USB, external screens and USB4
ss=0
for u in $R/sys/bus/usb/devices/usb*; do s="$(read1 "$u/speed")"; [ "${s:-0}" -ge 5000 ] 2>/dev/null && ss=$((ss + 1)); done
[ -d $R/sys/bus/usb/devices ] && [ "$ss" -eq 0 ] && [ -n "$(ls $R/sys/bus/usb/devices 2>/dev/null)" ] && add usb partial measured "no USB controller offers SuperSpeed (5 Gb/s or more)"
ext=""
for c in $R/sys/class/drm/card*-DP-* $R/sys/class/drm/card*-HDMI-*; do
  [ "$(read1 "$c/status")" = connected ] && ext="${c##*/} $(read1 "$c/modes")"
done
[ -n "$ext" ] && add external-display works measured "${ext#card?-} connected"
for d in $R/sys/bus/thunderbolt/devices/*; do
  case "${d##*/}" in [0-9]-[0-9]*) add usb4 works measured "USB4 device $(read1 "$d/device_name") at $(read1 "$d/rx_speed") Gb/s x $(read1 "$d/rx_lanes")" ;; esac
done

# Camera and video decoder. A built-in camera is a sensor on I2C behind Qualcomm's camera
# subsystem, named like "ov02c10 10-0036"; a USB one shows as a video device of its own
cams=""; dec=""; camss=""
for v in $R/sys/class/video4linux/video*; do
  n="$(read1 "$v/name")"
  case "$n" in
    *iris*|*venus*|*[Dd]ecoder*) dec="$n" ;;
    msm_vfe*|msm_csi*|*camss*) camss=1 ;;
    *[Cc]amera*|*[Ww]ebcam*|*UVC*) cams="$n" ;;
  esac
done
for v in $R/sys/class/video4linux/v4l-subdev*; do
  n="$(read1 "$v/name")"
  printf '%s' "$n" | grep -qE '^[a-z0-9]+ [0-9]+-[0-9a-f]{4}$' && cams="${n%% *} sensor"
done
if command -v cam >/dev/null 2>&1; then
  listed="$(timeout -k 2 10 cam -l 2>/dev/null | grep -E '^[0-9]+:' | head -n 1)"
  [ -n "$listed" ] && cams="${listed#*: }"
fi
if [ -z "$cams" ] && [ -n "$camss" ]; then add camera broken measured "the camera subsystem is up, but no sensor was found"
elif [ -z "$cams" ] && [ "$LAPTOP" = 1 ]; then add camera broken measured "no camera device"; fi
[ -z "$dec" ] && add video-decode broken measured "no iris or venus video decoder"

# Fans and temperatures
fan=""; temps=0
for h in $R/sys/class/hwmon/hwmon*; do
  for f in "$h"/fan*_input; do [ -r "$f" ] && fan="$(read1 "$h/name") $(read1 "$f") rpm"; done
done
for z in $R/sys/class/thermal/thermal_zone*/temp; do [ -r "$z" ] && temps=$((temps + 1)); done
[ -n "$fan" ] && add fans works measured "$fan; $temps temperature sensors"

# A fingerprint reader on USB, by maker
fp=""
for u in $R/sys/bus/usb/devices/*; do
  case "$(read1 "$u/idVendor" 2>/dev/null)" in 27c6|06cb|04f3|1c7a|10a5|2808|298d) fp="$(read1 "$u/idVendor"):$(read1 "$u/idProduct")" ;; esac
done

# --- Guided test: with the owner ----------------------------------------------
guided() {
  if [ -z "$TTY" ]; then say "The guided test needs a terminal to ask you questions; run it in one, not through a pipe without a terminal."; return; fi
  say ""
  say "Guided test. Answer y (works), p (partly), n (doesn't work), s (skip) or a (this model doesn't have it)."
  say ""

  verdict() { # verdict check "question" [evidence]
    local check="$1" q="$2" ev="${3:-}" note=""
    while :; do
      ask "$q [y/p/n/s/a]" || return
      case "$REPLY" in
        y|Y) add "$check" works owner "$ev"; return ;;
        p|P) ask "  What doesn't work?" && note="$REPLY"; add "$check" partial owner "$ev" "$note"; return ;;
        n|N) ask "  What happens?" && note="$REPLY"; add "$check" broken owner "$ev" "$note"; return ;;
        a|A) add "$check" absent owner "$ev"; return ;;
        s|S|"") return ;;
      esac
    done
  }

  # Speakers and microphones: a tone on each side while the microphones listen, then the owner's
  # own voice recorded and played back, so they hear for themselves whether the microphones work
  rms() { od -An -v -t d2 -w2 "$1" 2>/dev/null | awk '{s+=$1*$1; n++} END {if (n) printf "%d", sqrt(s/n); else print 0}'; }
  # For how many quarter seconds of a recording the 880 Hz test tone is most of the sound, the highest
  # share it reaches in percent, and how loud it is then. Short windows, because the speakers' and the
  # microphones' clocks differ a little, which smears the tone out of a long measurement; and the tone
  # has to be most of the sound, so room noise and the click of the microphones starting don't count.
  tone() {
    od -An -v -t d2 -w2 "$1" 2>/dev/null | awk 'BEGIN { c = 2 * cos(2 * 3.14159265358979 * 880 / 48000) }
      { e += $1 * $1; s = $1 + c * s1 - s2; s2 = s1; s1 = s; n++
        if (n == 12000) {
          share = e ? 100 * (s1 * s1 + s2 * s2 - c * s1 * s2) / (n / 2 * e) : 0
          if (share >= 50) { heard++; level += sqrt(e / n) }
          if (share > best) best = share
          e = 0; s1 = 0; s2 = 0; n = 0
        } }
      END { printf "%d %d %d", heard, best, (heard ? level / heard : 0) }'
  }
  if command -v speaker-test >/dev/null 2>&1 && command -v arecord >/dev/null 2>&1 && command -v aplay >/dev/null 2>&1; then
    say "Sound: set the volume to about half. Two tones of three seconds play, left then right, while the microphones listen."
    ask "Press Enter to start (s to skip)." && [ "$REPLY" != s ] && {
      for side in 1 2; do
        timeout -s KILL 6 arecord -q -d 3 -f S16_LE -r 48000 -c 1 -t raw "$TMP/tone$side.raw" 2>/dev/null &
        rec=$!
        sleep 0.2
        # speaker-test doesn't stop on a polite signal, so it is killed after three seconds
        timeout -s KILL 3 speaker-test -c 2 -s "$side" -t sine -f 880 -l 1 >/dev/null 2>&1
        wait "$rec"
      done
      read -r lheard lbest llevel <<<"$(tone "$TMP/tone1.raw")"
      read -r rheard rbest rlevel <<<"$(tone "$TMP/tone2.raw")"
      loop="the microphones heard the left tone for $lheard quarter seconds (up to $lbest% of the sound, level $llevel), the right for $rheard (up to $rbest%, level $rlevel)"
      say "  $loop"
      verdict speakers "Did you hear a tone from the left speaker, then from the right, cleanly?" "$loop"

      say "Microphones: after you press Enter, say a few words. The probe records 4 seconds and plays them back to you."
      ask "Press Enter, then speak (s to skip)." && [ "$REPLY" != s ] && {
        timeout -s KILL 8 arecord -q -d 4 -f S16_LE -r 48000 -c 1 "$TMP/voice.wav" 2>/dev/null
        tail -c +45 "$TMP/voice.wav" >"$TMP/voice.raw" 2>/dev/null
        voice="$(rms "$TMP/voice.raw")"
        say "  Playing it back (level $voice)."
        # Players can hang after the last sample on some machines, so playback is cut off after seven seconds
        if command -v pw-play >/dev/null 2>&1; then timeout -s KILL 7 pw-play "$TMP/voice.wav" >/dev/null 2>&1
        else timeout -s KILL 7 aplay -q "$TMP/voice.wav" >/dev/null 2>&1; fi
        verdict microphones "Did you hear your own voice played back clearly (p if it was very quiet)?" "voice level $voice; $loop"
      }
      # Skipped the voice test: the tones the microphones picked up still say something
      if [ -z "${STATE_OF[microphones]:-}" ]; then
        if [ "$lheard" -ge 3 ] || [ "$rheard" -ge 3 ]; then add microphones works measured "$loop"
        elif [ "$(rms "$TMP/tone1.raw")" -eq 0 ] && [ "$(rms "$TMP/tone2.raw")" -eq 0 ]; then add microphones broken measured "the microphones recorded only silence"; fi
      fi
    }
  else
    verdict speakers "Do both speakers play music cleanly?"
    verdict microphones "In a voice recorder or a call, do the built-in microphones pick you up?"
  fi
  verdict headphones "If you have wired headphones at hand, plug them in: do they play (a if it has no jack)?"

  # Camera: one frame, checked for being more than black
  if [ -n "$cams" ] && command -v cam >/dev/null 2>&1; then
    say "Camera: taking one frame (the light may flash)."
    # cam takes its optional values attached, and a PPM file holds only 24-bit pixels
    timeout -k 3 20 cam -c 1 --capture=8 --stream pixelformat=BGR888 --file="$TMP/frame-#.ppm" >/dev/null 2>&1 ||
      timeout -k 3 20 cam -c 1 --capture=8 --stream pixelformat=RGB888 --file="$TMP/frame-#.ppm" >/dev/null 2>&1
    frame="$(ls "$TMP"/frame-*.ppm 2>/dev/null | tail -n 1)"
    if [ -n "$frame" ]; then
      mean="$(tail -c 200000 "$frame" | od -An -v -t u1 | awk '{for (i = 1; i <= NF; i++) {s += $i; q += $i * $i; n++}} END {m = s / n; printf "%d %d", m, sqrt(q / n - m * m)}')"
      set -- $mean
      if [ "${1:-0}" -gt 8 ] && [ "${2:-0}" -gt 4 ]; then
        add camera works measured "frame from $cams, brightness $1, contrast $2"
        verdict camera "Open a camera page in Chromium (like webcamtests.com). Does the picture look right, colours included?" "frame brightness $1, contrast $2"
      else add camera broken measured "frame from $cams is black (brightness ${1:-0})"; fi
    else
      verdict camera "The camera gave no frame here. Does it work in Chromium?" "cam captured nothing from $cams"
    fi
  else
    verdict camera "Does the webcam give a picture in Chromium?"
  fi

  # Sleep: wait for the owner to close and open the lid, then see what came back
  if [ -r $R/sys/power/suspend_stats/success ]; then
    say "Sleep: close the lid, wait 20 seconds, open it and log back in. The probe waits up to 3 minutes."
    ask "Press Enter, then close the lid (s to skip)." && [ "$REPLY" != s ] && {
      before="$(read1 $R/sys/power/suspend_stats/success)"; fails="$(read1 $R/sys/power/suspend_stats/fail)"
      waited=0
      while [ "$(read1 $R/sys/power/suspend_stats/success)" = "$before" ] && [ "$(read1 $R/sys/power/suspend_stats/fail)" = "$fails" ] && [ $waited -lt 180 ]; do sleep 2; waited=$((waited + 2)); done
      if [ "$(read1 $R/sys/power/suspend_stats/fail)" != "$fails" ]; then
        add suspend broken measured "the sleep failed (suspend_stats fail went from $fails to $(read1 $R/sys/power/suspend_stats/fail))"
      elif [ "$(read1 $R/sys/power/suspend_stats/success)" = "$before" ]; then
        verdict suspend "The laptop didn't sleep within 3 minutes. Did closing the lid do anything?"
      else
        sleep 15
        back=""
        [ -n "$wif" ] && [ "$(read1 "$R/sys/class/net/$wif/operstate")" != up ] && back+="Wi-Fi not reconnected after 15 s; "
        for rp in $R/sys/class/remoteproc/remoteproc*; do
          case "$(read1 "$rp/name")" in *adsp*) [ "$(read1 "$rp/state")" = running ] || back+="adsp $(read1 "$rp/state") after waking; " ;; esac
        done
        if [ -n "$back" ]; then add suspend partial measured "woke, but: ${back%; }"
        else add suspend works measured "slept and woke; Wi-Fi and the audio DSP came back"; fi
        verdict suspend "Did it wake to the lock screen without trouble?" "$(read1 $R/sys/power/mem_sleep)"
      fi
    }
  fi

  # What only a person can tell. A check the probe already measured as working isn't asked about,
  # and every question can be answered from the chair, or skipped when it hasn't been tried.
  say ""
  say "A few questions. Answer s for anything you haven't tried."
  unmeasured() { [ "${STATE_OF[$1]:-}" != works ]; }
  [ -n "$KB" ] && verdict keyboard "Do the letter, number and arrow keys all type on the built-in keyboard?" "$KB"
  [ -n "$TP" ] && verdict touchpad "On the touchpad, do moving the pointer, clicking and scrolling with two fingers work?" "$TP"
  [ -n "$TS" ] && verdict touchscreen "Tap the screen with a finger: does the tap land where you touch (a if it has no touchscreen)?" "$TS"
  [ -n "$bl" ] && verdict backlight "Press the brightness keys now: does the screen get darker and brighter?" "$bl"
  unmeasured display && verdict display "Does the desktop fill the built-in screen and look sharp?"
  verdict disk-unlock "When you started the laptop today, did you see the disk password prompt and type into it with the built-in keyboard (a if the disk isn't encrypted)?"
  verdict usb "Have USB sticks or other devices worked in every USB port you've tried?"
  unmeasured external-display && verdict external-display "If you've plugged in an external screen: did it show a picture?"
  unmeasured charging && verdict charging "With its own charger plugged in, does the battery percentage go up?"
  [ -n "$fp" ] && verdict fingerprint "If you've set up the fingerprint reader: does it unlock with your finger?" "USB $fp"
  unmeasured bluetooth && verdict bluetooth "If you've used Bluetooth headphones or a mouse: do they pair and stay connected?"
  verdict installer "If you installed Omarchy on it yourself: did the installer start from the USB stick with keyboard and touchpad working?"
}
[ "$GUIDED" = 1 ] && guided

# --- Report -------------------------------------------------------------------
HW=""
for kv in "maker:$MAKER" "product:$PRODUCT" "family:$FAMILY" "sku:$SKU" "board:$BOARD" "bios:$BIOS"; do
  k="${kv%%:*}"; v="${kv#*:}"; [ -n "$v" ] && HW+="${HW:+,}\"$k\":$(json_str "$v")"
done
MODE=quick; [ "$GUIDED" = 1 ] && MODE=guided
[ -n "$AGENT" ] && [ "$GUIDED" = 0 ] && MODE=quick
BODY="{\"mode\":\"$MODE\""
[ -n "$MODEL" ] && BODY+=",\"model\":$(json_str "$MODEL")"
[ -n "$AGENT" ] && BODY+=",\"agent\":$(json_str "$AGENT")"
BODY+=",\"compatible\":$(json_str "$COMPATIBLE"),\"kernel\":$(json_str "$KERNEL"),\"os\":$(json_str "$OS")"
BODY+=",\"hardware\":{$HW},\"chids\":[$CHIDS],\"packages\":{$PKGS},\"results\":["
first=1
for r in "${RESULTS[@]+"${RESULTS[@]}"}"; do [ $first = 1 ] && first=0 || BODY+=","; BODY+="$r"; done
BODY+="]}"

if [ "$JSON_ONLY" = 1 ]; then printf '%s\n' "$BODY"; exit 0; fi

say "Results"
for r in "${RESULTS[@]+"${RESULTS[@]}"}"; do
  c="$(printf '%s' "$r" | sed -E 's/.*"check":"([^"]*)".*/\1/')"
  s="$(printf '%s' "$r" | sed -E 's/.*"state":"([^"]*)".*/\1/')"
  h="$(printf '%s' "$r" | sed -E 's/.*"how":"([^"]*)".*/\1/')"
  e="$(printf '%s' "$r" | sed -nE 's/.*"evidence":"(([^"\\]|\\.)*)".*/\1/p')"
  printf '  %-17s %-8s %-9s %s\n' "$c" "$s" "($h)" "$e" >&2
done
say ""
[ ${#RESULTS[@]} -eq 0 ] && { say "Nothing could be measured. Is this a Snapdragon PC running Linux?"; exit 1; }

if [ "$SEND" = ask ]; then
  # Without a terminal there's nobody to ask: an agent runs it this way, and --no-send stops it
  if ask "Send this to $API for the ${MAKER:-} ${PRODUCT:-} page? [Y/n]"; then
    case "$REPLY" in n|N|no|No) SEND=no ;; *) SEND=yes ;; esac
  else
    SEND=yes
  fi
fi
[ "$SEND" = yes ] || { say "Not sent."; exit 0; }

STATE_DIR="${XDG_STATE_HOME:-$HOME/.local/state}/arewearmyet"
mkdir -p "$STATE_DIR"
if [ ! -s "$STATE_DIR/token" ]; then
  ( umask 077; head -c 32 /dev/urandom | base64 | tr '+/' '-_' | tr -d '=\n' >"$STATE_DIR/token" )
fi
TOKEN="$(cat "$STATE_DIR/token")"
printf '%s' "$BODY" >"$TMP/body.json"
response="$(curl -fsS -X POST "$API/api/runs" -H 'content-type: application/json' -H "x-machine-token: $TOKEN" --data-binary "@$TMP/body.json" 2>&1)" || {
  say "Sending failed: $response"; exit 1; }
run="$(printf '%s' "$response" | sed -nE 's/.*"run":"([^"]*)".*/\1/p')"
page="$(printf '%s' "$response" | sed -nE 's/.*"page":"([^"]*)".*/\1/p')"
printf '%s\n' "$run" >"$STATE_DIR/last-run"
say "Sent as $run. Thank you!"
if [ -n "$page" ]; then
  say "It shows on $page"
  # Not every terminal opens a link on a click, so the probe offers to
  if [ -n "$TTY" ] && command -v xdg-open >/dev/null 2>&1 && ask "Open that page in your browser? [Y/n]"; then
    case "$REPLY" in n|N|no|No) ;; *) xdg-open "$page" >/dev/null 2>&1 & ;; esac
  fi
fi
if [ -z "$page" ]; then
  # The board doesn't know the machine: the form to add it, with what identifies it filled in
  urlencode() {
    local s="$1" out="" c i
    for (( i = 0; i < ${#s}; i++ )); do
      c="${s:i:1}"
      case "$c" in [a-zA-Z0-9.~_-]) out+="$c" ;; $'\n') out+="%0A" ;; *) out+="$(printf '%%%02X' "'$c")" ;; esac
    done
    printf '%s' "$out"
  }
  ids="$(printf '%s' "$CHIDS" | tr -d '"' | tr ',' '\n' | tr ':' ' ')"
  form="https://github.com/birkskyum/arewearmyet/issues/new?template=model.yml"
  form+="&model=$(urlencode "${MAKER:-} ${FAMILY:-$PRODUCT}")&model-number=$(urlencode "${PRODUCT:-}")&probe=$run"
  [ -n "$ids" ] && form+="&hwids=$(urlencode "$ids")"
  say "The board doesn't know this machine yet, so the run shows on no page."
  say "Add it with this form, which has its name and hardware IDs filled in:"
  say "  $form"
  say "If it is on the board under another name, run again with --model <id> from $API/api/models."
fi
say "To see exactly what was stored: $API/api/runs/$run"
