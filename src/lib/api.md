# API reference

Everything arewearmyet.com shows about a computer can be read over this API, and everything `hw-test` sends goes through it. An agent that helps an owner should read the [guide for agents](https://arewearmyet.com/agents.md) first. This page is the reference behind it.

## Basics

- The base address is `https://arewearmyet.com/api`. `GET /api` lists every route as JSON.
- Requests and answers are JSON, with one exception, a note's log, which is plain text.
- Nothing needs an account or a key. Reading is open to everyone, from a browser on another site too.
- An error comes back as `{"error": "..."}` with a sentence that says what to send instead.
- Text for agents is at [/docs/api.md](https://arewearmyet.com/docs/api.md).

### The machine token

A computer that sends something names itself with a random string it keeps, in the header `X-Machine-Token`. It is 32 to 128 characters of letters, digits, `-` and `_`. `hw-test` makes one on first use and keeps it in `~/.local/state/arewearmyet/token`.

The site stores only the token's SHA-256. The token does three things. A computer's newer results replace its older ones in the counts, so one owner who tests ten times counts once. Only that computer can add results to its own run. Only that computer can mark its own problem solved.

A token is never returned by any route. Keep it on the computer and out of logs and pull requests.

### Words used everywhere

| Field | Values |
| --- | --- |
| `state` | `works`, `partial`, `broken`, `untested`, `absent` (the computer doesn't have the part) |
| `how` | `measured` (a test saw it), `owner` (the owner saw or heard it), `agent` (an agent's reading of logs or output) |
| `mode` | `quick` (the read-only check), `guided` (the full test with the owner), `agent` (sent by an agent) |
| note `kind` | `problem`, `workaround`, `upstream`, `note` |
| note `status` | `open`, `solved`. Only problems have one |

A model's id looks like `lenovo-yoga-slim-7x-14q8x9` and a run's like `RUN-85CBV653X8`. A fix is `FIX-` and a note `NOTE-` followed by ten characters. A check id such as `speakers` comes from `GET /api/checks`.

### What is taken out of text

Every text field is cleaned on arrival, whatever sent it. Hardware addresses become `xx:xx:xx:xx:xx:xx`, a home folder becomes `/home/user`, and what follows `serial`, `s/n` or `uuid` is removed. Longer text (a note's body, a log) also loses network addresses in private ranges, addresses with a port, packet sources and destinations, disk and partition IDs, the machine ID and network names made from a hardware address. A log also loses the names of Bluetooth devices. `hw-test` does the same cleaning before it sends, with the computer's name and the owner's names added.

### Limits

| What | Limit |
| --- | --- |
| Sending (every `POST` below except the firmware routes) | 30 requests a minute per address, then `429` |
| A run | 256 KB, 100 results |
| Results added to a run | 128 KB, 100 results |
| A fix | 64 KB, 40 measured changes |
| A note or an answer | 1 MB, body of 20 to 8000 characters, 8 links, 4 logs of 200 KB each |
| A result's `note` | 600 characters |
| A result's `evidence` | 400 characters |

A field longer than its limit is cut. A request or a log over its size is refused. When reports are switched off for maintenance, every sending route answers `503`.

Answers to reads may be cached for a short while. That is five minutes for checks, models and owners, one minute for a model, and half a minute for notes and fixes.

### Links

A fix's `url` and a note's `links` must be `https` links to where Linux, its firmware, Mesa and the distros review changes. Those are github.com, gitlab.com, lore.kernel.org, patchwork.kernel.org, git.kernel.org, gitlab.freedesktop.org, lists.freedesktop.org, patchwork.freedesktop.org, lists.infradead.org, patchew.org, bugs.launchpad.net, codeberg.org, bugzilla.kernel.org, gitlab.archlinux.org and arewearmyet.com. Any other link is refused with `400`.

## Reading

### `GET /api`

The list of routes, with a line on each and the address of this page.

```json
{
  "name": "Are we ARM yet?",
  "docs": "https://arewearmyet.com/docs/api",
  "guide": "https://arewearmyet.com/agents.md",
  "routes": [{ "method": "GET", "path": "/api/checks", "does": "Every check: ..." }]
}
```

### `GET /api/checks`

What every check means. Read it before reporting, since a result names a check by its `id`.

```json
{
  "states": ["works", "partial", "broken", "untested", "absent"],
  "how": ["measured", "owner", "agent"],
  "groups": [{ "id": "sound", "title": "Sound" }],
  "checks": [
    {
      "id": "speakers",
      "group": "sound",
      "title": "Speakers",
      "works": "Every built-in speaker plays, at a normal volume, without crackling.",
      "probe": "Whether a sound card with a playback device exists, besides HDMI and USB ones.",
      "person": "Play music and say whether both sides (and the tweeters, if it has them) play cleanly.",
      "debug": "Sound runs through the audio DSP. In order: ..."
    }
  ]
}
```

| Field | Meaning |
| --- | --- |
| `works` | What has to be true for the check to count as working |
| `probe` | What the quick check measures by itself. Missing when nothing can be measured |
| `person` | What the owner does or says |
| `debug` | Where to look when it fails, in the order worth trying |
| `optional` | `true` for a part many computers don't have. Such a check is in a model's `wanted` only where the model is known to have the part |

### `GET /api/models`

Every computer model with a page: the Snapdragon PCs that start through UEFI, the Chromebooks and Googlebooks, and the NVIDIA machines. An NVIDIA machine has no `chips` or `compatibles`. A Chromebook or Googlebook has the same fields as a Snapdragon PC. Omarchy's install image doesn't start on one, so its `bootable` is `works`, `workaround`, `problems`, `untested` (a Linux image starts, nobody tried Omarchy) or `notyet`.

```json
{
  "models": [
    {
      "id": "lenovo-yoga-slim-7x-14q8x9",
      "vendor": "Lenovo",
      "name": "Yoga Slim 7x",
      "generation": "x1",
      "chips": ["X1E-78-100", "X1P-64-100"],
      "modelNumbers": ["14Q8X9", "83ED"],
      "compatibles": ["lenovo,yoga-slim7x"],
      "outcome": "works",
      "bootable": "stable",
      "checksWrittenUp": 16,
      "page": "https://arewearmyet.com/models/lenovo-yoga-slim-7x-14q8x9",
      "api": "https://arewearmyet.com/api/models/lenovo-yoga-slim-7x-14q8x9"
    }
  ]
}
```

`bootable` is the most stable Omarchy channel whose install image starts the model by itself (`stable`, `rc`, `edge`), or a word for what is in the way (`needs-deviceTree`, `needs-ids`, `workaround`, `problems`). `outcome` says how far it gets. `works` means an owner reached the desktop, `should` means the pieces are there and nobody has confirmed it, and `workaround`, `problems`, `manual` and `none` name what is in the way. `checksWrittenUp` counts the checks with a finding for Omarchy.

### `GET /api/models/<id>`

One model, with everything its page shows. `404` names the routes that find the right id. An NVIDIA machine's record is smaller. It has no `upstream`, `variants` or `reports`, and its `boots` holds `outcome`, the `kernel` channel that starts it and `linux` (whether Linux itself has the drivers, or only Omarchy's kernel).

| Field | Meaning |
| --- | --- |
| `id`, `vendor`, `name`, `generation`, `form`, `released`, `chips`, `modelNumbers`, `page` | Which computer it is |
| `boots` | Whether Omarchy starts on it. `bootable` and `outcome` as in the list, `pieces` with the channel that has the device tree, the hardware IDs and the firmware, and `omarchy` with the pull requests they came in |
| `upstream` | Where the same three pieces stand in Linux, systemd and linux-firmware |
| `variants` | One entry per chip or board variant, with its `compatible`, its device tree file and the firmware files it needs |
| `checks` | Every check with what the team wrote up, as `finding`. A finding has a `state`, a `note`, the `source` it comes from, `basis` (whose result it is, Omarchy's or another distro's) and often `where`, `version`, `checked` and a `quote`. A finding with `basis: maker-spec` comes from the maker's specification. With `state: absent` it says the model doesn't have the part, and with `state: untested` that the maker lists the part and nobody has tested it on Omarchy |
| `wanted` | The checks with no finding for Omarchy on this model yet, for the parts the model has. Start there |
| `owners` | What owners' computers reported, see below |
| `fixes` | Fixes linked to the model, as `GET /api/fixes` returns them |
| `notes` | Up to 100 notes on the model, newest first, as `GET /api/notes` returns them |
| `details`, `links`, `reports` | The team's own text on the model, its threads elsewhere, and its boot reports |

`owners` counts each computer once per check, by its newest result.

```json
{
  "machines": 1,
  "checks": {
    "webgpu": {
      "machines": 1,
      "states": { "works": 1 },
      "how": { "measured": 1 },
      "latest": [
        {
          "state": "works",
          "how": "measured",
          "note": null,
          "evidence": "Chromium offers WebGPU on qualcomm adreno-7xx",
          "kernel": "7.3.0-72-omarchy",
          "created": "2026-11-19T15:44:41.050Z",
          "run": "RUN-85CBV653X8"
        }
      ],
      "fixed": { "machines": 1, "from": "7.3.0-59-omarchy", "to": "7.3.0-72-omarchy", "since": "2026-11-19T15:44:41.050Z" }
    }
  }
}
```

`latest` holds up to five results, newest first. `fixed` and `regressed` appear when a computer's result turned from failing to working or the other way, with the kernels on both sides. One rule differs from "newest wins". A measurement that finds a part working doesn't take back a failure the owner reported. The owner's result stands until the owner tests that part again.

### `GET /api/match`

Which model a computer is. Give what the computer says about itself, as query parameters.

| Parameter | From |
| --- | --- |
| `chid` | A hardware ID with its type, as `systemd-analyze chid` numbers them, like `3:8073dbed-501f-5f5e-a619-4cdd9c00e865`. Repeat the parameter or separate with commas, most specific first |
| `compatible` | The running device tree's compatible strings, from `/proc/device-tree/compatible` |
| `product`, `sku`, `family`, `vendor` | The firmware's strings, from `/sys/class/dmi/id/` |

Hardware IDs decide first, then the device tree, then a model number found in the firmware strings.

```
curl 'https://arewearmyet.com/api/match?compatible=lenovo,yoga-slim7x'
```

```json
{
  "model": "lenovo-yoga-slim-7x-14q8x9",
  "by": "compatible",
  "vendor": "Lenovo",
  "name": "Yoga Slim 7x",
  "generation": "x1",
  "boot": "uefi",
  "family": "arm-dragon-msft",
  "page": "https://arewearmyet.com/models/lenovo-yoga-slim-7x-14q8x9",
  "kernels": { "stable": "7.3-3.1", "rc": "7.3-3.1", "edge": "7.3-71" }
}
```

`by` is `hardware IDs`, `compatible` or `model number`. `family` is the line a pull request for the computer opens with. It is `arm-dragon-msft` for a Snapdragon PC that came with Windows, `arm-dragon-google` for a Chromebook or Googlebook, and `arm-nvidia` for an NVIDIA machine. An NVIDIA machine starts through ACPI (`"boot": "acpi"`), has no hardware ID list or device tree, and is found by the model number in `product`, `sku` or `family` together with `vendor`. `kernels` is the `linux-omarchy` version on each of Omarchy's channels when the site was last built, to tell a computer that is behind. A computer the board doesn't know gets `{"model": null, "by": null, "hint": "..."}` with status `200`.

### `GET /api/owners`

Every model's owners' results in one answer, and the computers that matched no model.

```json
{
  "models": { "lenovo-yoga-slim-7x-14q8x9": { "machines": 1, "checks": {} } },
  "unmatched": [
    { "hardware": { "maker": "...", "product": "..." }, "compatible": "...", "chids": [], "machines": 1, "runs": 2, "latest": "2026-11-19T15:44:41.050Z" }
  ]
}
```

Each model's value has the shape of `owners` in `GET /api/models/<id>`.

### `GET /api/runs/<id>`

One run as it is stored, for its owner to check what was sent.

```json
{
  "id": "RUN-85CBV653X8",
  "model": "lenovo-yoga-slim-7x-14q8x9",
  "compatible": "lenovo,yoga-slim7x",
  "created": "2026-11-19T15:44:41.050Z",
  "mode": "guided",
  "kernel": "7.3.0-72-omarchy",
  "os": "Omarchy",
  "packages": { "linux-omarchy": "7.3-72", "linux-omarchy-dtbs": "7.3-48", "mesa": "1:26.2.7-1", "hw-test": "0.19.0" },
  "hardware": { "maker": "LENOVO", "product": "83ED", "family": "Yoga Slim 7 14Q8X9", "sku": "...", "board": "LNVNB161216", "bios": "NHCN62WW" },
  "agent": null,
  "chids": ["0:5315f3e1-1fa9-5592-bf05-75e8124d1f8a"],
  "results": [
    { "check": "gpu", "state": "works", "how": "measured", "note": null, "evidence": "Vulkan draws on Adreno X1-85 (turnip Mesa driver)" }
  ]
}
```

### `GET /api/fixes`

What one pull request changed, over every computer that measured it. Takes `?url=`, the link the fix was sent with. The example shows the shape, with made-up values.

```
curl 'https://arewearmyet.com/api/fixes?url=https://github.com/omacom/omarchy-pkgs/pull/1234'
```

```json
{
  "url": "https://github.com/omacom/omarchy-pkgs/pull/1234",
  "machines": 2,
  "improved": ["speakers"],
  "worse": [],
  "checks": { "speakers": { "improved": 2, "unchanged": 0, "worse": 0 } },
  "fixes": [
    {
      "id": "FIX-K7Q2M9XH4P",
      "model": "lenovo-yoga-slim-7x-14q8x9",
      "check": "speakers",
      "url": "https://github.com/omacom/omarchy-pkgs/pull/1234",
      "note": "Routes the right woofer to its own output",
      "created": "2026-11-19T16:02:11.000Z",
      "changes": [
        {
          "check": "speakers",
          "from": { "state": "partial", "how": "measured", "evidence": "left heard at -47 dB, right not heard", "note": null },
          "to": { "state": "works", "how": "measured", "evidence": "left -47 dB, right -48 dB", "note": null }
        }
      ],
      "versions": { "before": { "kernel": "7.3.0-72-omarchy", "packages": {} }, "after": { "kernel": "7.3.0-72-omarchy", "packages": {} } }
    }
  ]
}
```

`improved` lists the checks that got better on at least one computer and worse on none. `worse` lists the checks that got worse on any. A link nobody has sent yet answers with zero computers and empty lists.

### `GET /api/notes`

Notes are what fits no check, such as a problem that takes a sequence of events, a way around one, or a pointer to work elsewhere. Newest first, 100 at most. The example shows the shape, with made-up values.

| Parameter | Meaning |
| --- | --- |
| `model` | A model id. Without it, notes on every model |
| `check` | A check id |
| `kind` | `problem`, `workaround`, `upstream` or `note` |
| `status` | `open` or `solved`. Gives problems only |
| `q` | Up to six words. Each has to be in the title or the text |

```json
{
  "notes": [
    {
      "id": "NOTE-4H7WQ2M9XK",
      "model": "lenovo-yoga-slim-7x-14q8x9",
      "check": "suspend",
      "kind": "problem",
      "title": "Wi-Fi gone after sleep",
      "status": "open",
      "body": "Slept about five minutes. Wi-Fi was gone on wake and came back after a restart.",
      "links": [],
      "kernel": "7.3.0-72-omarchy",
      "os": "Omarchy",
      "packages": { "linux-omarchy": "7.3-72" },
      "agent": null,
      "created": "2026-11-19T17:30:02.000Z",
      "replies": 1,
      "logs": [{ "name": "kernel.log", "url": "https://arewearmyet.com/api/notes/NOTE-4H7WQ2M9XK/logs/kernel.log" }],
      "url": "https://arewearmyet.com/api/notes/NOTE-4H7WQ2M9XK",
      "page": "https://arewearmyet.com/note?id=NOTE-4H7WQ2M9XK"
    }
  ]
}
```

In a list, `body` is cut after 600 characters and `replies` is a count. An unknown `model`, `check`, `kind` or `status` is refused with `400`.

### `GET /api/notes/<id>`

One note in full. `replies` is the list of its answers, oldest first, each with the same fields.

### `GET /api/notes/<id>/logs/<name>`

One log of a note, as `text/plain`. The address comes from the note's `logs`.

## Sending

Every route here takes JSON, counts towards the limit of 30 a minute, and cleans the text it stores. `hw-test` uses these routes for you. The test itself sends runs, `hw-test report` sends one result, `hw-test verify after` sends a fix and `hw-test note` sends notes. See the [hw-test reference](https://arewearmyet.com/docs/hw-test).

### `POST /api/runs`

A run is what one computer found at one time. Needs `X-Machine-Token`.

```
curl -X POST https://arewearmyet.com/api/runs \
  -H 'content-type: application/json' \
  -H "x-machine-token: $(cat ~/.local/state/arewearmyet/token)" \
  -d '{
    "mode": "agent",
    "agent": "Claude Code",
    "kernel": "7.3.0-72-omarchy",
    "os": "Omarchy",
    "compatible": "lenovo,yoga-slim7x",
    "results": [
      { "check": "hibernate", "state": "broken", "how": "agent", "evidence": "/sys/power/state: freeze mem", "note": "The kernel offers no hibernation." }
    ]
  }'
```

| Field | |
| --- | --- |
| `mode` | Required. `quick`, `guided` or `agent` |
| `results` | Required. 1 to 100 of `{check, state, how, note?, evidence?}`, each check once |
| `model` | A model id. Left out, the computer's own strings decide, as in `GET /api/match` |
| `chids` | Up to 40 hardware IDs with their type, like `3:8073dbed-...` |
| `compatible` | The device tree's first compatible string |
| `hardware` | `maker`, `product`, `family`, `sku`, `board`, `bios`, from the firmware. Never a serial number |
| `kernel`, `os`, `agent` | What ran, and which agent sent it |
| `packages` | Versions of `linux-omarchy`, `linux-omarchy-dtbs`, `linux`, `linux-firmware`, `linux-firmware-qcom`, `mesa`, `systemd`, `omarchy`, `hyprland`, `pipewire`, `alsa-ucm-conf`, `bluez`, `libcamera`, `hw-test`, and on NVIDIA machines `linux-omarchy-nvidia-open`, `nvidia-utils`, `libva-nvidia-driver`. Other names are dropped |

Answers `201`.

```json
{
  "run": "RUN-85CBV653X8",
  "model": "lenovo-yoga-slim-7x-14q8x9",
  "stored": 1,
  "url": "https://arewearmyet.com/api/runs/RUN-85CBV653X8",
  "page": "https://arewearmyet.com/models/lenovo-yoga-slim-7x-14q8x9"
}
```

A computer that matches no model is kept all the same. `model` and `page` are then `null`, a `note` says so, and the run shows on no page until the model is on the board. Results with the state `untested` are stored and never counted.

Say honestly how each result was found. The pages show `measured`, `owner` and `agent` apart, so an agent's reading never looks like a measurement.

### `POST /api/runs/<id>/results`

More results for a run, from the computer that made it. Needs the same `X-Machine-Token`. The body is `{"results": [...]}` as above. A check the run already has is replaced. Answers `{"run", "stored", "url"}`, or `404` when the run isn't this computer's.

### `POST /api/fixes`

A link to where a fix is reviewed, for a model, with what it changed.

| Field | |
| --- | --- |
| `model` | Required. A model id |
| `url` | Required. The pull request or patch, see Links above |
| `check` | The check it is about, when it is about one |
| `note` | What the change is, in a line |
| `changes` | 1 to 40 of `{check, from, to}`. `from` and `to` are `{state, how?, evidence?, note?}`, the result before and after. Needs `X-Machine-Token`, so each computer counts once |
| `before`, `after` | `{kernel, packages}` on each side. Kept only with `changes` |

Answers `201` with `{"fix", "url", "page"}`. A computer that measures the same pull request again replaces what it said before. Without `changes` the fix is a plain link on the model's page, and no token is needed.

### `POST /api/notes`

Writes a note. Needs `X-Machine-Token`.

```
curl -X POST https://arewearmyet.com/api/notes \
  -H 'content-type: application/json' \
  -H "x-machine-token: $(cat ~/.local/state/arewearmyet/token)" \
  -d '{
    "model": "lenovo-yoga-slim-7x-14q8x9",
    "check": "suspend",
    "kind": "problem",
    "title": "Wi-Fi gone after sleep",
    "body": "Slept about five minutes. Wi-Fi was gone on wake and came back after a restart.",
    "logs": [{ "name": "kernel.log", "text": "..." }]
  }'
```

| Field | |
| --- | --- |
| `kind` | Required. `problem`, `workaround`, `upstream` or `note` |
| `title` | Required. 8 to 140 characters |
| `body` | Required. 20 to 8000 characters. Longer output goes in `logs` |
| `model` | A model id. May be left out by a computer that has sent a run, which is known by its model |
| `check` | The check it is about, when it is about one |
| `links` | Up to 8 links, see Links above |
| `logs` | Up to 4 of `{name, text}`. A name is lowercase letters, digits, dots and dashes, like `kernel.log`. A log is 200 KB at most |
| `kernel`, `os`, `packages`, `agent` | As for a run |

Answers `201` with `{"note", "model", "url", "page"}`.

### `POST /api/notes/<id>/replies`

Answers a note. Needs `X-Machine-Token`. Takes `body`, and optionally `links`, `logs`, `kernel`, `os`, `packages` and `agent`. The model and the check are the note's own. An answer can't be answered itself, and the error names the note to answer instead. Answers `201` with `{"note", "of", "model", "url", "page"}`, where `of` is the note that was answered.

### `POST /api/notes/<id>/status`

Marks a problem solved, or open again. The body is `{"status": "solved"}` or `{"status": "open"}`. Only the computer that wrote the problem can do it, with its `X-Machine-Token`. Anyone else gets `404` and answers the note instead.

## For the site's keepers

### `GET /api/review`

The newest 100 runs, fixes and notes, hidden ones included. Needs `Authorization: Bearer` with the admin token. `?before=` takes a time to page back from.

### `POST /api/review/hide`

Hides or shows a report. Needs the admin token. The body is `{"kind": "run" | "fix" | "machine" | "note", "id": "...", "hidden": true | false}`. Hiding a machine hides everything it sent.

## Firmware drop

These routes belong to the Omarchy Arm Installer. With the owner's yes, it sends the firmware files it finds in the laptop's Windows drivers, so the team can work out what a model needs. The bucket has no public address and nothing here lists or returns a stored file. The limit is 300 requests a minute per address, and the routes answer `503` when the drop is closed.

### `POST /api/firmware/missing`

Takes `{"sha256": [...]}` with up to 40 lowercase SHA-256 hashes and answers `{"missing": [...]}`, the ones the drop doesn't hold yet. Only those need sending.

### `PUT /api/firmware/files/<sha256>`

The file as the request body, 95 MB at most, with `Content-Length`. A body that doesn't match the hash in the address is refused. Answers `201` with `{"stored": true}`, or `{"stored": false}` when the file was there already.

### `POST /api/firmware/uploads/<sha256>`

Starts sending a file over 95 MB and up to 1 GB, in parts. Takes `{"size": <bytes>}` and answers `201` with `{"uploadId"}`.

### `PUT /api/firmware/uploads/<sha256>/<upload>/<part>`

One part as the request body. Parts are numbered from 1 to 100, each the same size but the last, at least 5 MiB and at most 95 MB. Answers `{"etag"}`.

### `POST /api/firmware/uploads/<sha256>/<upload>/complete`

Takes `{"parts": [{"partNumber", "etag"}]}`, joins the parts, checks the SHA-256 and stores the file. Answers `201` with `{"stored": true}`.

### `POST /api/firmware/reports`

The list of every file with its driver package. Takes `{"installer", "machine", "files"}`. `installer` is its version. `machine` has `manufacturer` and `product`, and may have `family`, `sku`, `baseboard` and `bios`. `files` is 1 to 5000 of `{folder, name, size, sha256}`. Answers `201` with `{"code": "FW-...", "known": false}`, the code the owner puts in a hardware report. The same computer sending the same files again gets its first code back with `"known": true`.
