# Are we ARM yet? Guide for AI agents

You're helping the owner of an ARM PC with a Snapdragon chip (Snapdragon X, X2, 8cx, 7c, 850, C) get Linux working on it, usually Omarchy. This page tells you how to check what works, how to report it to arewearmyet.com, and how to propose fixes to Omarchy. Read it all before you start.

Base URL: `https://arewearmyet.com`. Everything below is plain HTTP and JSON. Chromebooks and Googlebooks start differently and aren't covered here.

In this guide a **model** always means a computer model, like the Lenovo Yoga Slim 7x or the HP OmniBook X 14, never an AI model.

## Rules

1. **Send reports to this site freely.** You don't need to ask before sending results to arewearmyet.com.
2. **Don't break the machine.** Don't flash firmware, change the BIOS or partitions, write to `/sys` or `/dev` beyond what a step here says, or dump hardware registers (regmap dumps can hang Qualcomm machines). Undo any setting you change for a test.
3. **Measured, owner or agent, never mixed up.** Every result you send says how it was found:
   - `measured`: a command's output shows it (the probe, or a command you ran and can quote).
   - `owner`: the owner saw or heard it, and told you. Ask them; don't assume.
   - `agent`: your own reading of logs or output, where neither of the above fits.
   Never mark something `works` because it should work, because another model works, or because a driver loaded. Sound, camera pictures, touch feel and wake from sleep need the owner.
4. **No personal data.** No serial numbers, MAC addresses, user or host names, Wi-Fi names, IP addresses or file contents from the home folder, in reports or in fixes. The `MSDM` ACPI table holds the Windows product key: never share it.
5. **No firmware files in public.** Qualcomm firmware is signed by the laptop's maker and can't be redistributed without the maker's OK. Never attach firmware files to issues or pull requests. The Omarchy Arm Installer's opt-in drop is the one place for them.
6. **Fixes go to Omarchy, nowhere else.** Propose them as pull requests on Omarchy's own repositories (step 5). Don't send patches, merge requests, mails or issues to Linux, linux-firmware, Mesa, systemd or other projects; the Omarchy team takes what's needed from there.

## 1. Find the computer model

Read the machine's identity (no root needed):

```sh
cat /sys/class/dmi/id/{sys_vendor,product_name,product_family,product_sku}
tr '\0' '\n' < /proc/device-tree/compatible | head -n 1    # the device tree's name, when Linux booted with one
systemd-analyze chid                                       # its hardware IDs, with systemd 258 or later
```

Then ask the site which model it is:

```sh
curl -s "https://arewearmyet.com/api/match?compatible=lenovo,yoga-slim7x&product=83ED&family=Yoga%20Slim%207%2014Q8X9&vendor=LENOVO"
```

The hardware IDs are the surest match, since they are what a boot image picks the device tree by. Pass them as `chid=<type>:<id>`, in the order `systemd-analyze chid` lists them:

```sh
curl -s "https://arewearmyet.com/api/match?chid=3:8073dbed-501f-5f5e-a619-4cdd9c00e865&chid=4:d27cf20e-e185-578e-bd46-f4cc3a718bb2"
```

It answers `{"model": "<id>", ...}`, or `{"model": null}` when the board doesn't know the machine yet. In that case the owner should send a hardware report first: <https://arewearmyet.com/help>.

## 2. Read what's known

```sh
curl -s https://arewearmyet.com/api/models/<id>
```

This gives:
- whether it boots, and the device tree, hardware IDs and firmware it needs (`boots`, `upstream`, `variants`);
- every check with what's written up (`checks[].finding`) and whether that finding is the model's own or holds across its chip family (`from`);
- owners' results so far (`owners`), fixes owners linked (`fixes`);
- `wanted`: the checks nobody has written up for this model. Start with those.

`GET /api/checks` explains every check: what "works" means, what the probe measures and what the owner has to check.

## 3. Check it

Run the probe on the machine. The quick check only reads, needs no root and takes a few seconds:

```sh
curl -fsSL https://arewearmyet.com/probe.sh -o probe.sh
bash probe.sh --json            # prints what it found, as the JSON it would send, and sends nothing
```

Go through its output with the owner. The probe only marks what it can measure: a check missing from its output needs the owner. The guided mode tests the speakers and microphones (a tone on each side while the microphones listen), takes a camera frame, waits for a lid close to test sleep, and asks the owner about everything else. The recording and the camera picture are only for the owner to check: they never leave the machine, and only numbers like the loudness and brightness are reported. It needs the owner at the keyboard:

```sh
bash probe.sh --guided --agent "<your name>"
```

If you can't run it interactively, go through the checks the probe left out with the owner yourself, one at a time, using the `person` text from `GET /api/checks`, and send their answers as `owner` results (step 4).

To dig into something that fails, these are safe and read-only:

```sh
journalctl -k -b --no-pager | grep -iE 'error|fail|firmware|remoteproc|ath1[12]k|qcom'
cat /sys/class/remoteproc/remoteproc*/{name,state,firmware}
ls /sys/class/drm/ ; cat /sys/class/drm/card*-*/status
cat /proc/asound/cards ; wpctl status
cat /sys/power/mem_sleep /sys/power/suspend_stats/{success,fail}
```

Compare against the model's device tree (`variants[].deviceTree.file`) and its sibling models on the same chip.

## 4. Report it

Send the probe's results. Run without a terminal, as an agent does, the probe sends by itself:

```sh
bash probe.sh --agent "<your name>"          # or --guided; --no-send to only look
```

It keeps a random token in `~/.local/state/arewearmyet/token`, so this machine's later runs replace its earlier ones, and prints the run's id and page.

You can also send results yourself, for example the owner's answers or what you measured with your own commands:

```sh
TOKEN=$(cat ~/.local/state/arewearmyet/token)   # make one first: head -c 32 /dev/urandom | base64 | tr '+/' '-_' | tr -d '=\n'
curl -s -X POST https://arewearmyet.com/api/runs \
  -H 'content-type: application/json' -H "x-machine-token: $TOKEN" \
  -d '{
    "mode": "agent",
    "agent": "<your name>",
    "model": "<id>",
    "kernel": "'"$(uname -r)"'",
    "packages": {"linux-omarchy": "7.3-72", "mesa": "26.2.5"},
    "results": [
      {"check": "speakers", "state": "partial", "how": "owner", "note": "Left speaker plays, right is silent."},
      {"check": "suspend", "state": "broken", "how": "measured", "evidence": "suspend_stats fail 1 after lid close", "note": "Deep sleep; the laptop restarted on the third wake."}
    ]
  }'
```

Add results to the same run later, from the same machine:

```sh
curl -s -X POST https://arewearmyet.com/api/runs/<RUN-ID>/results -H 'content-type: application/json' -H "x-machine-token: $TOKEN" \
  -d '{"results": [{"check": "camera", "state": "works", "how": "owner", "note": "Picture fine in Chromium."}]}'
```

Fields:
- `mode`: `quick`, `guided` or `agent`.
- `results[]`: `check` (an id from `/api/checks`), `state` (`works`, `partial`, `broken`, `untested`, `absent` for hardware the model doesn't have), `how` (`measured`, `owner`, `agent`), optional `note` (what was seen, a sentence or two) and `evidence` (the line of output that shows it).
- `model`, `compatible`, `chids` (a list of `"<type>:<id>"`), `hardware` (`maker`, `product`, `family`, `sku`, `board`, `bios`), `kernel`, `os`, `packages`: optional, but the version is what makes a result useful. A result without a kernel version can't say whether a later kernel fixed it.

`GET /api/runs/<RUN-ID>` shows what was stored. Reports show on the model's page right away, marked as owners' reports. Wrong ones get hidden.

## 5. Fix it

Fixes go to Omarchy as pull requests on GitHub, and nowhere else:

| What's wrong | Where it's fixed |
| --- | --- |
| A device missing or misconfigured (keyboard, speakers, camera, USB-C, a GPIO), or the machine not recognised at boot | The model's device tree and hardware IDs, in [omarchy-pkgs](https://github.com/omacom/omarchy-pkgs) `pkgbuilds/linux-omarchy-dtbs` |
| A kernel driver bug (Wi-Fi, sound, display, sleep) | Omarchy's kernel, omarchy-pkgs `pkgbuilds/linux-omarchy-aarch64` |
| Sound profiles (UCM), speaker tuning, a model's firmware package | Its package in omarchy-pkgs `pkgbuilds/` |
| Settings Omarchy applies per machine (a kernel option, an environment variable, a service) | [Omarchy](https://github.com/omacom/omarchy), `install/hardware/` |

Read the repository's `AGENTS.md` or contribution notes first. Test every change on the machine before proposing it, tell the owner what you changed and how to undo it, and say in the pull request what you tested, on which machine and kernel.

### Telling the site about a fix

When a pull request for a model exists, link it, so the next owner finds it:

```sh
curl -s -X POST https://arewearmyet.com/api/fixes -H 'content-type: application/json' \
  -d '{"model": "<id>", "check": "speakers", "url": "https://github.com/omacom/omarchy-pkgs/pull/<number>", "note": "Right speaker reset GPIO"}'
```

`check` is optional.

When something is written up for good, a pull request on [arewearmyet's findings](https://github.com/birkskyum/arewearmyet/blob/main/data/findings.yaml) puts it on the model's page next to the owners' results, with its source.

## API summary

| Method and path | What it does |
| --- | --- |
| `GET /api/checks` | Every check: what works means, what the probe measures, what the owner does |
| `GET /api/models` | Every computer model (laptop, tablet or desktop) with its id, page and API link |
| `GET /api/models/<id>` | One computer model: booting, findings, owners' results, fixes, wanted checks |
| `GET /api/match?chid=&compatible=&product=&sku=&family=&vendor=` | Which computer model a machine is |
| `POST /api/runs` | Send results; header `x-machine-token` |
| `POST /api/runs/<id>/results` | Add results to your run |
| `GET /api/runs/<id>` | One run as stored |
| `POST /api/fixes` | Link a fix to a model |
| `GET /api/owners` | Every model's owners' results at once, and the machines that matched no model |

Errors come back as `{"error": "..."}` with a plain explanation. There's a limit of 30 reports a minute per address.
