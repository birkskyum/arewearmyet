# hw-test reference

`hw-test` is the hardware test for ARM computers on Omarchy. It checks the screen, keyboard, sound, camera, Wi-Fi, sleep and the rest, some of it by itself and some of it with you, and puts the results on your model's page on arewearmyet.com. When a part doesn't work, it hands that part to your AI agent with what was measured and where a fix goes.

This page lists every command. To get started, [How to help](https://arewearmyet.com/help) is shorter. An agent reads the [guide for agents](https://arewearmyet.com/agents.md), which says what to do with these commands and in which order.

## Install and update

```sh
omarchy-mise-install github:birkskyum/arewearmyet hw-test
hw-test
```

`hw-test` says at its start when a newer release is out, and `hw-test update` gets it. `hw-test version` prints the one you have. Releases are on [GitHub](https://github.com/birkskyum/arewearmyet/releases), built for Linux on ARM and on x86.

On a distro other than Omarchy, the probe does the quick check: `curl -fsSL https://arewearmyet.com/probe.sh | bash`.

## The test

| Command | What it does |
| --- | --- |
| `hw-test` | The guided test. It opens with where the computer stands, runs the quick check, then goes through the parts with you |
| `hw-test quick` | The quick check alone. It reads what the system says about each part, takes a few seconds, asks nothing and changes nothing |
| `hw-test auto` | Every test that needs nobody: sound, graphics, WebGL and WebGPU, Wi-Fi and Bluetooth, video decoding, temperatures. It heats the computer for a few minutes |
| `hw-test manual` | Every test done with you: keyboard, touchpad, touchscreen, headphones, camera, USB ports, external screen, lid, sleep and a few questions |
| `hw-test STEP...` | One part alone, or a few, like `hw-test camera` or `hw-test wifi bluetooth`. Only those parts are reported |
| `hw-test status` | The latest result of every check on this computer. Nothing is run |
| `hw-test facts` | The devices the test found to work with. Nothing is sent |

### Steps

| Word | What is tested | Takes you |
| --- | --- | --- |
| `keyboard` | Every key of the built-in keyboard, then the volume and brightness keys | yes |
| `keys` | The volume and brightness keys alone | yes |
| `touchpad` | Moving, clicking, scrolling | yes |
| `touchscreen` | Touch and pen, where the computer has them | yes |
| `sound` | Speakers and microphones, then headphones | for the headphones |
| `speakers`, `microphone` | Tones played on each side and listened to by the built-in microphones | no, but it is heard |
| `voice` | Your voice recorded and played back to you | yes |
| `headphones` | Whether sound moves to plugged-in headphones | yes |
| `camera` | A picture from the webcam, shown to you | yes |
| `usb` | Each USB port, with something plugged in | yes |
| `screen` | An external screen | yes |
| `lid` | Closing and opening the lid | yes |
| `sleep` | Sleep and wake, and what the battery lost | yes |
| `questions` | What no test can see, like the disk password prompt at startup | yes |
| `graphics`, `vulkan` | Drawing with the graphics chip | no |
| `browser`, `webgl`, `webgpu` | WebGL and WebGPU in the browser | no |
| `wireless`, `wifi`, `bluetooth` | Wi-Fi and Bluetooth | no |
| `video` | Hardware video decoding | no |
| `temperatures`, `fans` | Temperature sensors under load, and the fan, which you confirm by ear when no sensor reports it | for the fan |

The picture the camera takes and the recordings the sound tests make stay on the computer and are deleted after their step.

### Results

Every check ends in one of five states.

| State | Meaning |
| --- | --- |
| works | The part does its job |
| partly | It works with a fault, like one silent speaker |
| broken | It doesn't work |
| not tested | Nothing was measured |
| absent | The computer doesn't have the part |

A result that works but looks off (one speaker much quieter than the other) is marked as worth a look, and can be handed to an agent like a failing one. The quick check only sees that a part is there and its driver came up, so its result never replaces what a closer test found.

## What is sent

A test sends its results to your model's page when it ends. With them go the model, the kernel and the versions of a dozen system packages. No serial number, address, name or file is sent, and no account is needed. The computer is told apart by a random token kept in `~/.local/state/arewearmyet/token`, of which the site stores only a hash.

| Option | What it does |
| --- | --- |
| `--no-send` | Shows the results and sends nothing |
| `--json` | Prints exactly what would be sent, and sends nothing |
| `--model ID` | The model's id on arewearmyet.com, for a computer that isn't recognised by itself |
| `--agent NAME` | The agent running the test, shown next to the run |

When the network isn't up yet, as right after waking, `hw-test` waits for it and tries again.

## When something doesn't work

| Command | What it does |
| --- | --- |
| `hw-test fix` | Asks which part your agent should look into, writes a brief with what was measured, where to look and where a fix goes, and opens your agent on it. Nothing is tested again |
| `hw-test fix speakers` | The same for the part you name |
| `hw-test brief [WHAT]` | Prints that brief, to give to any agent yourself |

An agent takes one part at a time, so a part that can't be fixed doesn't hold up the others. The opening screen of `hw-test` offers the same for a part that fails. It shows the conversation your agent already has on a part, and sends the next part into it.

The agent works in `~/Work/hw-bringup` (or `~/hw-bringup`). `hw-test` uses the agent you have set up on Omarchy. Where T3 Code is installed, the brief goes into a thread there. An agent already running in T3 Code, herdr, Codex or Claude Code is continued in the conversation it has.

| Setting | What it does |
| --- | --- |
| `HW_TEST_AGENT` | The command that starts your agent, which gets the brief as its last argument, like `foot -e claude` |
| `HW_TEST_TERMINAL` | The command that opens a terminal window for a step |
| `HW_TEST_T3_MODEL` | The model a T3 Code thread starts on, as `provider/model` |

## Notes

A note holds what fits no check, such as a freeze that takes a sequence of events, a way around a problem, or a pointer to work elsewhere. Notes show on the model's page, where other owners and their agents read and answer them.

```sh
hw-test note            # asks for the part, what happened, and whether you restarted since
hw-test note wifi       # names the part at once
```

Everything can also be given on the command line, as agents do.

```sh
hw-test note --kind problem --check suspend --log previous \
  --title "Restarts when woken with a dock plugged in" --body-file what-happens.md
```

| Option | What it does |
| --- | --- |
| `--kind` | `problem`, `workaround`, `upstream` or `note` |
| `--check ID` | The check it is about, when it is about one |
| `--title`, `--body`, `--body-file` | The note in a line, and its text |
| `--link URL` | A link to an issue, a pull request or a patch. Can be given several times |
| `--log` | Attaches a log, up to four. `kernel` is this boot's kernel log, `previous` the boot before, `boot-3` the third boot before this one, and a file name attaches that file |
| `--reply NOTE-ID` | Answers a note |
| `--solved NOTE-ID` | Marks your own problem solved |
| `--json`, `--no-send` | Print or show what would go, logs included, and send nothing |

A log is cleaned before it leaves the computer. Addresses, serial numbers, disk IDs, the computer's name and your names are taken out.

## For agents

These commands let an agent run the test without the owner passing results back and forth. The [guide for agents](https://arewearmyet.com/agents.md) says when to use each.

| Command | What it does |
| --- | --- |
| `hw-test status --json` | Every check's latest result on this computer as data, whoever ran the test |
| `hw-test quick --json` | The quick check as JSON. Nothing is sent |
| `hw-test auto --no-send` | The tests that need nobody. Runs without a terminal, without the sound test |
| `hw-test speakers --no-send --play` | The sound test without a terminal. `--play` says the owner agreed to the tones |
| `hw-test ask STEP...` | Opens a step that takes the owner in a window on their desktop, waits up to half an hour, and prints the result as JSON |
| `hw-test report --check ID --state works\|partial\|broken --note TEXT` | Sends one result that no test measured. `--how owner` (the default) is what the owner saw, `--how agent` a reading of logs. `--evidence` holds the line of output that shows it |

`hw-test status --json` gives an object with one entry per check.

```json
{
  "speakers": {
    "check": "speakers",
    "state": "works",
    "how": "measured",
    "evidence": "by the microphones at half volume: left speaker heard at -47 dB, right speaker heard at -54 dB",
    "kernel": "7.3.0-72-omarchy",
    "when": "2026-11-19T17:15:18+01:00"
  }
}
```

`shallow: true` marks a result of the quick check, which saw the part and not its work.

### Measuring a change

| Command | What it does |
| --- | --- |
| `hw-test verify before` | Takes a snapshot of where the computer stands, before a change. One still in use isn't replaced unless `--new` says so |
| `hw-test verify after` | After testing again, prints what the change did, check by check, as a block for a pull request |
| `hw-test verify after --pr URL --title TEXT` | Records the same on the model's page, tied to the pull request. `--check ID,ID` gives those checks a row even when they work on both sides |

### Trying a device tree change

| Command | What it does |
| --- | --- |
| `hw-test tree start` | Fetches the sources Omarchy's device tree package builds from, with its patches, into `./tree` |
| `hw-test tree build` | Compiles the tree as the package does and writes the change to `./tree.diff` |
| `hw-test tree boot` | Makes a boot image and a boot menu entry of its own, `linux-omarchy-hwtest`, set to start once by itself at the next restart. Needs the owner's password, asked in a window |
| `hw-test tree status` | Says whether the test tree or Omarchy's own is running |
| `hw-test tree patch "arm64: dts: qcom: BOARD: WHAT" --body-file WHY` | Writes the change as the one patch the package takes |
| `hw-test tree remove` | Takes the test entry and its image away |

The usual boot entry stays the default and is never touched.

### Coming back after a restart

| Command | What it does |
| --- | --- |
| `hw-test resume "WHAT"` | Before a restart. At the owner's next login the agent is brought back once, in the same conversation where its program allows, and told WHAT |
| `hw-test resume --show` | Says what is left for after the restart |
| `hw-test resume --cancel` | Takes it back |

## Files

`hw-test` keeps its state in `~/.local/state/arewearmyet` (or under `XDG_STATE_HOME`).

| File | What it holds |
| --- | --- |
| `token` | The computer's random token |
| `results.json` | The latest result of every check, which `hw-test status` shows |
| `last-run` | The id of the last run sent |
| `before.json` | The snapshot of `hw-test verify before` |
| `handed.json`, `threads.json` | Which parts went to an agent, and the conversation each is in |
| `resume.json` | What is left for after a restart |
| `pending.json` | Results that couldn't be sent yet. Kept for a week |

Deleting the folder makes the computer a new one to the site.

## Exit codes

`0` when the command did its work, `1` when it couldn't, and `2` for a command, step or option it doesn't know.
