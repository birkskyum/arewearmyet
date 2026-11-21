# The hardware test

`hw-test` is the hardware test for ARM computers on Omarchy. It checks the screen, keyboard, sound, camera, Wi-Fi, sleep and the rest, some of it by itself and some of it with your help, and puts the results on your model's page on arewearmyet.com. When a part doesn't work, it hands that part to your AI agent with what was measured and where a fix goes.

The first half of this page is for running the test. The second half lists every command. An agent reads the [guide for agents](https://arewearmyet.com/agents.md), which says what to do with these commands and in which order.

## Run it

```sh
omarchy-mise-install github:birkskyum/arewearmyet hw-test
hw-test
```

It opens on a menu of what it can test on your computer, with where each part stands. A full round takes about a quarter of an hour.

- **By itself** it tests the speakers and microphones (it plays test tones and listens to them), graphics, Wi-Fi and Bluetooth, the network cable, video decoding, and temperatures under load.
- **With you** it tests the keyboard, touchpad, touchscreen, headphones, camera, USB ports, an external screen, the lid and sleep.
- **The results** are shown to you, and sent to the model's page unless you say no. A recording or a camera picture is only for you to check. It never leaves the computer and is deleted when its step ends.

`hw-test` says at its start when a newer release is out, and `hw-test update` gets it. `hw-test version` prints the one you have. Releases are on [GitHub](https://github.com/birkskyum/arewearmyet/releases), built for Linux on ARM and on x86.

## Commands

| Command | What it does |
| --- | --- |
| `hw-test` | The guided test. It opens with where the computer stands, runs the quick check, then takes you through the parts |
| `hw-test quick` | The quick check alone. It reads what the system says about each part, takes a few seconds, asks nothing and changes nothing |
| `hw-test auto` | Every test that runs by itself, which is sound, graphics, WebGL and WebGPU, Wi-Fi and Bluetooth, the network cable, video decoding and temperatures. It heats the computer for a few minutes |
| `hw-test manual` | Every test that asks you to do something, which is the keyboard, touchpad, touchscreen, headphones, camera, USB ports, an external screen, the lid, sleep and a few questions |
| `hw-test STEP...` | One part alone, or a few, like `hw-test camera` or `hw-test wifi bluetooth`. Only those parts are reported |
| `hw-test status` | The latest result of every check on this computer. Nothing is run |
| `hw-test facts` | The devices the test found to work with. Nothing is sent |
| `hw-test changes` | What was changed on this computer for its hardware, as far as files show it. That is the kernel, its modules and its options, your own settings files for kernel modules, device rules and sound, browser switches that differ from Omarchy's, firmware and sound profile files that no package holds, and changed files of hardware packages. It also lists what this computer has recorded with `--needs`. It is for an agent to judge. Nothing is sent. `--json` prints it as data, with `sent` (what a report names), `settings` (each file's `path` and what it `does`), `placed`, `altered`, `packages`, `treeDiff` and `recorded` |

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
| `sleep` | Sleep and wake. On a laptop you close the lid. A computer without a lid is put to sleep by the test, and you wake it | yes |
| `questions` | What no test can see, like whether a USB stick worked in every port. The disk password prompt is asked about only where the quick check couldn't measure it | yes |
| `graphics`, `vulkan` | Drawing with the graphics chip | no |
| `browser`, `webgl`, `webgpu` | WebGL and WebGPU in the browser | no |
| `wireless`, `wifi`, `bluetooth` | Wi-Fi and Bluetooth | no |
| `ethernet` | The network socket, by reaching the router through a cable that is plugged in | no |
| `video` | Hardware video decoding | no |
| `temperatures`, `fans` | Temperature sensors under load, and the fan, which you confirm by ear when no sensor reports it | for the fan |

A step shows only where the computer has the part. A mini PC or desktop gets no keyboard, touchpad, lid or battery step, and its screen is tested in the `screen` step.

### Results

Every check ends in one of five states.

| State | Meaning |
| --- | --- |
| works | The part does its job |
| partly | It works with a fault, like one silent speaker |
| broken | It doesn't work |
| not tested | Nothing was measured |
| absent | The computer doesn't have the part |

A result that works but looks off (one speaker much quieter than the other) is marked as worth a look, and can be handed to an agent like a failing one. The quick check only sees that a part is there and its driver came up, so its result never replaces what a closer test found, on the computer or on the site. A part the quick check finds gone does replace it. The disk password prompt is the exception. On a laptop with an encrypted disk the quick check reads from the kernel's log when the built-in screen and keyboard came up at this boot and when the disk opened. A screen that was up first showed the prompt, and one that came up afterwards left it dark. That is a measurement of what happened, so it counts as a test, and it is taken again at every run.

The keyboard step lets you go on without pressing every key. When you pressed fewer than half of them, it records nothing, and what was found before stands.

## Letting your agent work on its own

By default an agent tells you before it restarts the computer, asks before it changes a system file or one of your settings files, and needs your password for every command that runs as root. For hardware work that means a prompt every few minutes.

`hw-test trust` changes that for a set time. Until that time:

- The agent restarts and changes system settings without asking first, and goes from step to step without checking in.
- `sudo` asks for no password, for the agent's own commands and for hw-test's steps that need root.

It still never touches your files, the disk's partitions or its encryption, and never flashes firmware or changes the BIOS. It still needs you for the power button, the disk password, and what only your eyes and ears can tell.

This gives every program you run root on the computer until the time is up, so use it while the hardware work is going on and not as a habit. It writes one file, `/etc/sudoers.d/hw-test-trust`, with the end time in the rule itself. After that time the rule does nothing, also if the computer was off when it ran out. `hw-test trust --end` takes it back early and removes the file. Nothing about it is sent to the site.

## What is sent

A test ends by showing its report, which is everything that would be sent, in one frame, and asks before it sends it. The report holds the results, the computer's model, BIOS version and hardware IDs, the kernel version and the versions of about fifteen system packages. It sends no serial number, address, name or file, and it needs no account. The computer is told apart by a random token kept in `~/.local/state/arewearmyet/token`, of which the site stores only a hash.

A few things about the computer itself go along, by name, because they change what the hardware does and are never ordinary use. Those are another kernel than Omarchy's, a kernel module from no repository, a kernel option beyond the ones Omarchy sets, and a test device tree. A kernel option is sent by its name, without its value. The report lists them under the computer's model and kernel, as in "Kernel option added on this computer: gpiolib_acpi.ignore_wake".

Your settings files are not sent, neither their names nor what is in them. Whether a part works because of something you changed takes reading those files, and that is your agent's job on your computer, when you have one. `hw-test changes` lists for it what was changed for the hardware, as far as files show it. The agent then records, for a part that works because of a change, one sentence saying which. That sentence shows under the part's result on the model's page as "works with a change made on this computer". With no agent, nothing more happens.

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
| `hw-test fix speakers --now` | Hands the part over while your agent is at work on another one, in place of what it has. Without `--now` nothing is changed then, and you are told what the agent is working on |
| `hw-test brief [WHAT]` | Prints that brief, to give to any agent yourself |

An agent takes one part at a time, so a part that can't be fixed doesn't hold up the others. The opening screen of `hw-test` offers the same for a part that fails. It shows the conversation your agent already has on a part, and sends the next part into it.

The agent works in `~/Work/hw-bringup` (or `~/hw-bringup`). One computer takes one agent at a time, since hardware work restarts it and two agents can't both do that. So a second part waits until the first is done. `hw-test` opens the agent you have set up on Omarchy, in a terminal, with the permission settings Omarchy starts it with. An agent already running in herdr, Codex, Claude Code or a T3 Code thread is continued in the conversation it has. New work goes into a T3 Code thread only where you ask for it with `HW_TEST_T3=1`.

| Setting | What it does |
| --- | --- |
| `HW_TEST_AGENT` | The command that starts your agent, which gets the brief as its last argument, like `foot -e claude` |
| `HW_TEST_TERMINAL` | The command that opens a terminal window for a step |
| `HW_TEST_T3` | `1` opens new work in a T3 Code thread. `0` leaves T3 Code out, a thread that is going there included. Unset, a thread that is going there is continued and nothing new is started there |
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
| `hw-test ask update` | Opens Omarchy's update in a window for the owner, who confirms it there, and prints how it ended as JSON, with whether it finished, which packages moved, whether a restart is due, and the last lines of its log. It refuses, with the reasons, where an update would do harm. That is while a test boot entry stands, when the running kernel is not Omarchy's own, or when a kernel, device tree or firmware package was built on the computer |
| `hw-test report --check ID --state works\|partial\|broken\|absent --note TEXT` | Sends a report of one result that no test measured. `absent` is for a part the computer turns out not to have. `--how owner` (the default) is what the owner saw, `--how agent` a reading of logs. `--evidence` holds the line of output that shows it |
| `hw-test report --check ID --needs TEXT` | Says that the part works on this computer because of a change made on it, in one sentence, like "The kernel option gpiolib_acpi.ignore_wake, which keeps the lid sensor from waking it." It shows under that computer's result and stays until taken back with `--needs none`. `--proved` says that switching the change off and measuring the part again showed it. `--how` is as above. It is refused for a part with no working result on this computer |

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
| `hw-test verify before [PART]` | Takes a snapshot of where the computer stands, before a change. Name the part the change is about, as in `hw-test verify before speakers`, and it gets a snapshot of its own, so one part's change is never measured against another's. One still in use isn't replaced unless `--new` says so |
| `hw-test verify after [PART]` | After testing again, prints what the change did, check by check, as a block for a pull request. With a part's name it measures against that part's snapshot |
| `hw-test verify after [PART] --pr URL --title TEXT` | Records the same on the model's page, tied to the pull request. `--check ID,ID` gives those checks a row even when they work on both sides |

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

### Trying a change in a kernel driver

| Command | What it does |
| --- | --- |
| `hw-test kernel start` | Fetches Omarchy's kernel source as the running kernel was built from it, with its patches, into `./kernel`. Nothing is built. It takes a few minutes and about 3 GB |
| `hw-test kernel build FOLDER...` | Builds the modules of a driver's folder, like `drivers/phy/qualcomm`, against the running kernel, in seconds, and writes the change to `./kernel.diff` |
| `hw-test kernel load MODULE...` | Loads a built module in place of the kernel's own, until the next restart. Needs the owner's password, asked in a window. A module that is in use and can't be taken out is left alone |
| `hw-test kernel status` | Says which test modules are loaded since the computer started |
| `hw-test kernel restore` | Takes the test modules out and loads the kernel's own again. A restart does the same |
| `hw-test kernel patch "SUBSYSTEM: WHAT" --body-file WHY` | Writes the change as one patch for the kernel's recipe |

Nothing here is written where the computer starts from, so a restart always brings back the kernel as Omarchy ships it. The kernel notes in its log that a module of its own making was loaded, which lasts until that restart. With Secure Boot's lockdown on, the kernel takes no unsigned module, and `load` then leaves the kernel's own in place. A driver that Omarchy's kernel has built in, or one that is in use from the first moment of startup, can't be tried this way and needs a whole test kernel, which the [guide for agents](https://arewearmyet.com/agents.md) describes.

### Coming back after a restart

| Command | What it does |
| --- | --- |
| `hw-test resume "WHAT"` | Before a restart. At the owner's next login the agent is brought back once, in the same conversation where its program allows, and told WHAT |
| `hw-test resume --show` | Says what is left for after the restart |
| `hw-test resume --cancel` | Takes it back |
| `hw-test trust [HOURS]` | Lets your agent work on its own for some hours, 8 unless you say, or for days, as in `hw-test trust 3d`, a week at most. It asks for your password once. An agent that runs it gets a window opened for you, where you decide |
| `hw-test trust --show` | Says whether it holds and until when. `--json` gives it as data |
| `hw-test trust --end` | Takes it back |

## Files

`hw-test` keeps its state in `~/.local/state/arewearmyet` (or under `XDG_STATE_HOME`).

| File | What it holds |
| --- | --- |
| `token` | The computer's random token |
| `results.json` | The latest result of every check, which `hw-test status` shows |
| `last-run` | The id of the last run sent |
| `before.json`, `before-PART.json` | The snapshots of `hw-test verify before`, one per named part |
| `handed.json`, `threads.json` | Which parts went to an agent, and the conversation each is in |
| `resume.json` | What is left for after a restart |
| `test-entry.json` | That a test boot entry is installed, since the boot partition can't be read without your password |
| `pending.json` | Results that couldn't be sent yet. Kept for a week |

Deleting the folder makes the computer a new one to the site.

## Exit codes

`0` when the command did its work, `1` when it couldn't, and `2` for a command, step or option it doesn't know.
