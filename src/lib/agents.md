# Are we ARM yet? Guide for AI agents

You're helping the owner of an ARM PC get Linux working on it, usually Omarchy on a Snapdragon laptop. This guide tells you how to measure what works, report it to arewearmyet.com, and fix what doesn't in Omarchy.

**Opened by `hw-test fix`, with a `BRIEF.md` in your folder?** Steps 1 to 4 are done for you: the computer is identified and measured, and the brief names the one part the owner picked. Read **Words** and **Rules**, then **step 5 (Fix it)** and **step 6 (Notes)**. A copy of this guide is in `GUIDE.md` beside the brief, and the brief gives the path of `hw-test` in case its name isn't found where you run.

**One part at a time, and stay on it.** Work on the brief's part and nothing else, until it works. When the device tree can't fix it, the fix lives somewhere else: a setting, a kernel patch that exists elsewhere, or a driver nobody has written yet. Step 5 has the order to try them in, under "Keep going". Hand the part back only for the reasons listed there, and then with what you tried and what the next step is. The brief lists what else fails or is worth a look. `hw-test brief <check>` prints the brief for one of those from the results already on the computer, without testing anything again, and the owner hands you one with `hw-test fix`, which asks them which.

Base URL: `https://arewearmyet.com`. Everything is plain HTTP and JSON. Chromebooks and Googlebooks start differently, and getting Linux onto them isn't covered here. Measuring, reporting and fixing work the same once it runs.

## Words

- **Model**: a computer model, like the Lenovo Yoga Slim 7x. Never an AI model.
- **Check**: one thing that can work or not, like `speakers` or `suspend`. Each has an id. `GET /api/checks` lists them.
- **State**: `works`, `partial` (shown to owners as "partly"), `broken`, `untested`, or `absent` for hardware the model doesn't have.
- **Device tree** (or just "tree"): the file that tells Linux what hardware the computer has and how it is wired. These computers don't describe themselves, so Omarchy ships one tree per model. A wrong or missing entry in it is the usual reason a part doesn't work.
- **Hardware IDs**: strings made from the maker and model names, by which the boot image picks the right tree.
- **Boot image**: one file holding the kernel, its first file system and the device trees. The **stock** one is what Omarchy installed. **Limine** is the boot menu that starts it.
- **Family**: `arm-dragon-msft` (Snapdragon computers that came with Windows), `arm-dragon-google` (Snapdragon Chromebooks and Googlebooks), `arm-nvidia`, `arm-apple`. In Omarchy's own code the two Snapdragon ones are `aarch64-qualcomm`.
- **The owner**: the person whose computer it is. They are at the keyboard when a test needs eyes, ears or hands.

## Rules

1. **The owner's data is the line.** Never touch their documents and other files, the disk's partitions or its encryption. Don't flash firmware or change the BIOS. Everything short of that is yours to work with, the boot menu and restarts included (step 5 says how to do it so the computer always comes back).
2. **Read freely, but not hardware registers.** Logs, `/sys`, `/proc` and the files under `/sys/kernel/debug/gpio` and `pinctrl` are safe to read. `regmap` dumps are not: they can hang these computers.
3. **Measured, owner or agent, never mixed up.** Every result says how it was found: `measured` (a test's output shows it), `owner` (they saw or heard it and told you), `agent` (your own reading of logs). Never call something `works` because it should work, because another model works, or because a driver loaded.
4. **No personal data.** No serial numbers, hardware or network addresses, user or host names, Wi-Fi names or file contents from the home folder, in reports, notes, issues or pull requests. The `MSDM` ACPI table holds the Windows product key: never share it.
5. **No firmware files in public.** They are signed by the computer's maker and can't be passed on. Never attach one to an issue or a pull request.
6. **Don't use the camera or the microphones yourself.** The tests that do are run by the owner, who sees what is recorded. One exception: the sound test, `hw-test speakers --no-send`, which you run yourself once the owner has said yes to it. Handing you the speakers or the microphones (`hw-test fix`, or the test's own screen) is that yes. Otherwise ask them once, and add `--play`. It plays tones at half volume for about twenty seconds, listens to them, keeps only how loud each one was and deletes the recording.
7. **Fixes, issues and pull requests go to Omarchy, and to no other project.** Don't post to Linux, linux-firmware, Mesa, libcamera, Chromium, systemd or anyone else, even if the owner agrees. The Omarchy team carries things on from there. What you learn about work going on elsewhere goes into a note of kind `upstream` (step 6).
8. **What needs no asking:** reading, measuring, reporting to this site, test boot entries, installing a test package under a name of its own, opening yourself again after a restart, and a pull request or an issue on Omarchy's repositories.
9. **What you tell the owner first:** a restart, since it closes what they have open (unless they said to restart as you need), and a pull request or issue, since it goes out under their GitHub account.
10. **What you ask the owner first:** changing a system file, replacing a stock package, changing a settings file in their home folder (like `~/.config/chromium-flags.conf`), and running `omarchy update`. Keep a copy of what you change and write into `PROGRESS.md` how to undo it.
11. **Root goes through `sudo`.** When it asks for a password you can't give, hand the owner the exact command to run in their own terminal.

## 1. Find the computer model

```sh
cat /sys/class/dmi/id/{sys_vendor,product_name,product_family,product_sku}
tr '\0' '\n' < /proc/device-tree/compatible | head -n 1    # the device tree's name for it
systemd-analyze chid                                       # its hardware IDs
```

Ask the site which model that is. The hardware IDs are the surest match:

```sh
curl -s "https://arewearmyet.com/api/match?compatible=lenovo,yoga-slim7x&product=83ED&family=Yoga%20Slim%207%2014Q8X9&vendor=LENOVO"
curl -s "https://arewearmyet.com/api/match?chid=3:8073dbed-501f-5f5e-a619-4cdd9c00e865&chid=4:d27cf20e-e185-578e-bd46-f4cc3a718bb2"
```

It answers with the model's id, its `family`, and `kernels`, the `linux-omarchy` version on each of Omarchy's channels. `{"model": null}` means the site doesn't know the computer yet: the owner sends a hardware report first, as <https://arewearmyet.com/help> says.

## 2. Read what's known

```sh
curl -s https://arewearmyet.com/api/models/<id>
```

- `checks[].finding`: what is written up about each check, with its `source` and the `quote` of the source it rests on. It often names the cause. `basis` says where it was seen: `omarchy` (or none, which means the same) is a result on Omarchy. `other-distro` and `tree-author` were seen on another distro or by the device tree's author on their own kernel, and `where` says which. Those show that a part can work, not that it does on Omarchy, so treat them as a hint and measure.
- `owners`: what `hw-test` measured on owners' computers. `fixes`: pull requests linked to the model, with what each changed. `notes`: problems and workarounds others left (step 6).
- `variants[].deviceTree.file`: the model's device tree. `upstream.acpiUrls`: its ACPI tables, where the right GPIO or address for a part is found.
- `wanted`: the checks nobody has written up. When you were sent to check a computer, not to fix one part, start there.

`GET /api/checks` explains every check: what "works" means, what is measured, what the owner does, and `debug`: where to look when it fails.

## 3. Measure

`hw-test` is the hardware test. Install it once, the way Omarchy installs its other tools:

```sh
omarchy-mise-install github:birkskyum/arewearmyet hw-test
hw-test quick --json
```

It says at its start when a newer release is out, and `hw-test update` gets it. Every command and option is in the [hw-test reference](https://arewearmyet.com/docs/hw-test.md).

| Command | What it does |
| --- | --- |
| `hw-test status` | The latest result of every check on this computer. Nothing is run |
| `hw-test quick --json` | The quick check: read-only, a few seconds, printed as JSON. Nothing is sent |
| `hw-test facts` | The devices the test found to work with |
| `hw-test auto --no-send` | Every test that needs nobody: graphics, WebGL and WebGPU, Wi-Fi and Bluetooth, video decoding, temperatures. About four minutes, and it heats the computer. Runs without a terminal |
| `hw-test graphics`, `wireless`, `video`, `temperatures` | One of those alone |
| `hw-test speakers --no-send` | The sound test: tones played and listened to by the microphones. You run it yourself, without a terminal, on the owner's yes (rule 6). Add `--json` for the result as data |
| `hw-test ask camera` (or `keyboard`, `touchpad`, `touchscreen`, `headphones`, `usb`, `screen`, `lid`, `sleep`) | A test that takes the owner: this opens it in a window on their desktop, waits until they are through, and prints the result as JSON. Tell them a window is coming. Never have them run a test and retype what it said |
| `hw-test status --json` | Every check's latest result on this computer as data, whoever ran the test |

- Add `--no-send` to look without reporting. Without it a run is sent to the model's page.
- The quick check only sees that a part is there and its driver came up. It never counts as the part working when a closer test exists.
- Measure before a change and after it.

On another distro, the probe does the quick check: `curl -fsSL https://arewearmyet.com/probe.sh -o probe.sh && bash probe.sh --json`.

## 4. Report

Running a test sends its results, which is all most reporting takes. For what no test measures:

```sh
hw-test report --check suspend --state partial --note "Wakes fine alone. With the dock plugged in it froze twice in five tries."
hw-test report --check wifi --state broken --how agent --evidence "ath12k_pci: failed to fetch board data" --note "The firmware has no entry for this model."
```

`--how` is `owner` by default (they saw or heard it and told you) or `agent` (your reading of logs). `--state absent` is for a part the computer turns out not to have, like a headphone jack on a laptop with only USB-C ports. It takes back a result recorded for it. A report holds only the checks you name. The site keeps each check's newest result per computer, so the other results stand.

Without `hw-test`, send the same as JSON. The token is the one `hw-test` keeps in `~/.local/state/arewearmyet/token`:

```sh
curl -s -X POST https://arewearmyet.com/api/runs \
  -H 'content-type: application/json' -H "x-machine-token: $(cat ~/.local/state/arewearmyet/token)" \
  -d '{"mode": "agent", "agent": "<your name>", "model": "<id>", "kernel": "'"$(uname -r)"'",
       "results": [{"check": "speakers", "state": "partial", "how": "owner", "note": "Left speaker plays, right is silent."}]}'
```

`results[]` takes `check`, `state`, `how`, and optional `note` and `evidence`. Results show on the model's page right away. Wrong ones get hidden.

### On a mini PC, desktop or all-in-one

`hw-test` tells these from laptops by the chassis type in the firmware, and tests the parts they have. It expects no battery, lid, built-in keyboard, touchpad or camera, and the model's page lists those under "Not on this model" in place of rows. The screen is tested in the `screen` step, sleep by putting the computer to sleep itself (`hw-test ask sleep`, and the owner wakes it with a key or the power button), and the network socket by `hw-test ethernet`, which reaches the router through a cable that is plugged in. A part the model turns out not to have, like a headphone jack, is recorded with `hw-test report --check ID --state absent`.

## 5. Fix it

### On an NVIDIA machine

An NVIDIA N1x or GB10 machine (family `arm-nvidia`) describes its own hardware through ACPI. It has no device tree, so `hw-test tree` and the device tree package are not for it, and it needs no hardware IDs or firmware package. A fault there is in a kernel driver, a system setting or an application's setting, and goes where the table below says for those. Measuring, reporting, `hw-test verify`, `hw-test resume` and notes work the same. The two Qualcomm processors (`adsp`, `cdsp`) are not on these machines. `hw-test` knows an N1x laptop and a GB10 desktop for what they are, checks the driver bound to the GPU, and decodes video on the GPU through NVDEC or VA-API. These paths were written without such a machine at hand, so if a test itself misbehaves on one, say so in a note and in an issue on [arewearmyet](https://github.com/birkskyum/arewearmyet/issues).

### Before you change anything

1. **Is the computer up to date?** Compare its `linux-omarchy` version with `kernels` from step 1. When it is behind its channel, ask the owner whether to update. On their yes, run `hw-test ask update`: it opens Omarchy's update in a window for them, where they confirm it, and prints how it ended as JSON (`finished`, `moved`, `restart`, the log's last lines), so they relay nothing. It refuses and gives the reasons in `why` where an update would do harm: a test boot entry stands, the running kernel is not Omarchy's own, or a kernel, device tree or firmware package was built on the computer. Tell the owner the reason then, and leave the decision with them. Never run the update any other way. Leave word with `hw-test resume "Updated. Measure the failing parts again."` before the restart, and measure again after it: the fault may be fixed already. A patch also has to fit Omarchy's current sources, not old ones.
2. **Is it known?** Read the check's `finding` and the model's notes (step 6). Search every model's notes too: a fault in the chip shows on all of them.
3. **Where does the fix live?** Almost always somewhere you can reach: the device tree, a setting, the kernel, or another of Omarchy's packages (the table below). "Keep going" says how to move from one to the next. Very little is out of reach, like firmware only the maker can sign.
4. **Take a snapshot:** `hw-test verify before <part>`, as in `hw-test verify before speakers`. It keeps where every check stands, with the kernel and package versions, to measure your change against. The name gives the part a snapshot of its own, so your work is not measured against the snapshot another agent took for another part. Use the same name with `verify after`. Take it once per change. A snapshot that is still in use is not replaced, so after a restart you go on from `PROGRESS.md` and don't take another. `--new` replaces it when you start on another change.
5. **Does the owner hear or see it differently than the test says?** Then record that first, since the snapshot holds what is recorded: `hw-test report --check speakers --state partial --note "The right side is clearly quieter."`

### What can harm the computer

A device tree switches power, so a wrong one can do more than fail. Three rules.

- **Supplies.** Give a rail the voltage the computer's own tables or its maker's tree give it, never a higher one. When you can't tell, leave the rail as the firmware set it.
- **Speakers.** A laptop's amplifiers can give its small speakers more power than they survive, and a protection algorithm holds that back. Don't bring up a speaker amplifier that no tree of this model or of a sibling describes, and don't raise a volume limit or take out a protection setting to make a test pass. Say what is missing and stop.
- **The way back.** Try a change only through `hw-test tree boot`, whose entry starts once and leaves the usual one alone. Don't write the usual boot image, the boot menu's settings or the firmware's boot order by hand.

### Where a fix goes

| What is wrong | Where it is fixed |
| --- | --- |
| A device missing or wrongly described (keyboard, speakers, camera, a USB-C port, a GPIO), or the computer not recognised at boot | Its device tree and hardware IDs: [omarchy-pkgs](https://github.com/omacom/omarchy-pkgs), `pkgbuilds/linux-omarchy-dtbs`. New tree patches go here even when older ones for the model sit in the kernel's recipe |
| A kernel driver's fault | Omarchy's ARM kernel: omarchy-pkgs, `pkgbuilds/linux-omarchy-aarch64`. (`pkgbuilds/linux-omarchy` is the x86 kernel. Don't copy that one.) |
| A firmware file the computer needs | The model's own package: omarchy-pkgs, `pkgbuilds/<maker>-<model>-firmware`. It says where the files come from and never carries files that may not be passed on |
| A sound profile | omarchy-pkgs, `pkgbuilds/alsa-ucm-conf` |
| A system setting for one model or a family (a kernel option, a service, a file under `/etc`) | [Omarchy](https://github.com/omacom/omarchy), branch `quattro`, `install/hardware/<family>/`, which runs as root before any user exists |
| A setting of an application in the user's home (Chromium's switches) | Omarchy: a `bin/omarchy-install-*` command called from `install/user/hardware/<family>/`, plus a migration for computers already installed. `bin/omarchy-install-video-decode` and `bin/omarchy-chromium-add-feature` show how |
| Something Omarchy has no place for, or that needs another project to change | No pull request. An issue on Omarchy and a note (step 6) |

- **omarchy-pkgs:** `README.md` says how packages are built (`bin/build --package <name> --arch aarch64`, which needs Docker or Podman), and `AGENTS.md` what a recipe change needs, like a `pkgrel` bump. Each package's own `README.md` explains its patches. Add a paragraph for yours there.
- **A device tree fix** is one patch in the kernel's format, named `00NN-arm64-dts-qcom-<board>-<what>.patch`, listed in the recipe's `source` and `b2sums`. It may only touch files under `arch/arm64/boot/dts/` and `include/dt-bindings/`. `hw-test tree patch` writes it for you (below).
- **Omarchy:** read `AGENTS.md` and `agents/skills/install-scripts.md` (and `migrations.md` when existing installs need the change). Fork, branch from `quattro`, run the changed `bin/` command from your checkout on the computer, run the area's tests in `test/shell.d/`, and open the pull request against `quattro`. Put a setting whose cause is the chip behind `omarchy-hw-aarch64-qualcomm`, and one model's behind `omarchy-hw-match`. In the pull request, call a platform setting a workaround and name the cause it works around.

### Keep going

The first thing you try often isn't the fix. That is no reason to stop. Go down this list, and write into `PROGRESS.md` what each step showed, so a later agent goes on from where you are.

1. **The device tree.** A node that is missing or wrong (next section).
2. **A setting.** A sound profile, a udev rule, a module option or a kernel option. Try a kernel option on the test boot entry, never on the usual one.
3. **A fix that exists elsewhere.** Search for the chip and the part on lore.kernel.org and in linux-next, in the kernels other distros carry for these computers (Ubuntu's, linux-surface, aarch64-laptops, postmarketOS), and in the maker's own kernel for Android or ChromeOS where there is one. Take the patch, keep its author and its link, and try it ("Trying a kernel change").
4. **A driver or an entry nobody has written.** Write it. The model's hardware report on this site has its ACPI tables, and often the Windows drivers' INF files and device list, which say which chip sits on which bus with which pins. A sibling chip's driver is the pattern. Start with the small ones: a panel's entry, a device ID a driver doesn't know yet, a quirk.
5. **Something in how the computer starts, or in another Omarchy package.** Find the smallest case that shows it, then fix it in that package's repository.

Tell the owner in a line when you move to the next step. Don't wait for an answer unless the rules say to ask.

Hand the part back only when one of these holds:

- A rule under "What can harm the computer" stands in the way.
- The next step needs the owner and they aren't there. Do everything that doesn't need them first.
- It needs something only the maker can give, like signed firmware.
- You went through the whole list. Then say for each step what you tried and what stopped it, and name the next thing worth trying.

Whatever the outcome, leave it as a note (step 6), with a link to your branch or pull request, the unfinished ones too.

### Trying a device tree change

`hw-test` does this from the same sources Omarchy's package builds from, with the patches the package lays over them, in the folder you work in:

```sh
hw-test tree start       # the sources in ./tree, under git, and Omarchy's packages in ./omarchy-pkgs
$EDITOR tree/arch/arm64/boot/dts/qcom/<this computer>.dts
hw-test tree build       # compiles it as the package does, and writes your change to ./tree.diff
hw-test tree boot        # makes a boot image and a boot menu entry of its own, set to start once by itself
```

- `tree start` needs the kernel's headers, which hold the sources: `sudo pacman -S linux-omarchy-headers`. It fetches Omarchy's packages for their patches, and says whether the sources, compiled unchanged, are the tree the computer starts with. When they are, a test image differs by your change and nothing else. When they are not, the computer is behind: update it first.
- `tree boot` needs the owner's password. Run it yourself: without a terminal it opens a window on their desktop where they type it, and waits until they are through. Tell them a window is coming. It writes what it does to `tree-boot.sh` first, for them to read. It builds an image from the stock kernel and your tree, and adds an entry called `linux-omarchy-hwtest` through the boot menu's own tool.
- The usual entry stays the default and is not touched. `tree boot` tells the computer's firmware to start the test entry at the next restart, one time, so nobody has to pick it: its last lines say whether that worked. The start after that is the usual one again, also when the test entry didn't come up. Where the firmware couldn't be told, the entry is listed under Omarchy in the boot menu, after the usual kernel, and the owner picks `linux-omarchy-hwtest` there. Either way tell them: if the screen stays black for a minute, hold the power button until the computer is off and start it again. After the restart, `hw-test tree status` says which tree is running. Check it before you measure.
- When Direct Boot is on, the computer starts its image without showing the boot menu, and `tree boot` stops and says so. The owner turns it off in the Omarchy menu under Setup, Direct Boot (or with `omarchy-setup-direct-boot`), and can turn it on again when you're done.
- After the restart, `hw-test tree status` says for certain which image is running, and `dtc -I fs -O dts /proc/device-tree | grep -A12 <node>` shows your change in the running tree.
- `hw-test tree remove`, run by the owner, takes the entry and its image away.

When the change works, write it as the patch the package takes:

```sh
hw-test tree patch "arm64: dts: qcom: <board>: <what it does>" --body-file why.md
```

It writes the whole change as one patch, with the next free number, under the owner's name as their git has it, and says what to add to the recipe in `./omarchy-pkgs`: the patch, its line in `source` and `b2sums`, a `pkgrel` bump and a paragraph in the README. `why.md` says what was wrong and how it was measured. Run it again after a further edit and it writes the patch anew. To open the pull request from that checkout: `gh repo fork --remote` in `./omarchy-pkgs`, a branch, and `gh pr create --draft`.

**Three things that can stop the computer from starting. Don't do them:**

- **Don't edit `/boot/limine.conf` by hand.** Omarchy's boot menu checks its config and files against stored checksums and refuses to start when they differ. Entries are added and removed through `limine-entry-tool`, which `hw-test tree boot` uses.
- **Don't install a rebuilt `linux-omarchy-dtbs`, and don't write under `/boot/dtbs`.** The stock boot image is rebuilt from what is there, so a bad tree would end up in the one image that has to work. The package's own build (`bin/build`) is for seeing that the recipe builds with your patch, which the pull request's checks do as well. What it makes is not for `pacman -U` on the owner's computer.
- **Don't run `omarchy kernel set`, `limine-update` or `limine-mkinitcpio` while a test is under way.** They rebuild the stock image.

### Trying a kernel change

**One driver, the quick way.** Most fixes touch one driver, and a driver that is a module builds alone, in seconds, against the kernel that is running. Get the kernel's source as Omarchy builds it, with its patches:

```sh
git clone --filter=blob:none --sparse https://github.com/omacom/omarchy-pkgs && cd omarchy-pkgs
git sparse-checkout set pkgbuilds/linux-omarchy-aarch64
rel=$(pacman -Q linux-omarchy | awk '{print $2}')   # like 7.3-72
P=pkgbuilds/linux-omarchy-aarch64/PKGBUILD
# the recipe as it was for the kernel that is running
for c in $(git log --format=%H origin/master -- $P); do
  git show $c:$P | grep -q "^pkgrel=${rel##*-}\$" && git show $c:$P | grep -q "^pkgver=${rel%-*}\$" && git checkout -q $c && break
done
cd pkgbuilds/linux-omarchy-aarch64 && makepkg -o --skippgpcheck --nodeps   # fetches and patches, builds nothing
```

That takes a minute or two and 2 GB. Then change the driver under `src/linux-*/` and build its folder:

```sh
cd src/linux-*/
make -C /usr/lib/modules/$(uname -r)/build M=$PWD/drivers/phy/qualcomm modules
```

The modules that come out carry the running kernel's version, so they load into it: `sudo rmmod <module>; sudo insmod ./<module>.ko` (the owner types the password, rule 11). That lasts until the next restart and changes nothing on disk. Measure with `hw-test`, and go on.

**A whole kernel, when the quick way doesn't reach.** A driver that is in use can't be taken out while it runs (the disk's, the screen's), one that loads in early boot sits in the boot image, and some changes are outside any module. Then build a test kernel, which takes a container and half an hour or more. Copy `pkgbuilds/linux-omarchy-aarch64`, give it a name of its own (`pkgbase=linux-omarchy-test`, and the same in `pkgname`), add your patch, build it and install the result. Every installed kernel package gets a boot image and a menu entry of its own, listed after the stock kernel, so the stock one stays the default. Remove the package when you're done. Check with `df -h /boot` that the boot partition has 400 MB free first: a full one breaks the next ordinary update.

**When it works**, the change goes into `pkgbuilds/linux-omarchy-aarch64` as a numbered patch. The recipe's README says how they are numbered and listed. A patch taken from elsewhere keeps its author and the link to where it came from.

### Restarting, and coming back

A restart ends your session. So does a test that may freeze the computer. Leave word for yourself first:

```sh
$EDITOR PROGRESS.md      # what you changed, which entry to boot, what to check next, how to undo it
hw-test resume "Booted linux-omarchy-hwtest with the right speaker's reset GPIO changed. Ask the owner to run hw-test speakers."
```

Then tell the owner to restart when it suits them, or restart yourself with `systemctl reboot` once they have said to (rule 9). A test boot entry starts by itself that one time (see above). At the owner's next login you are brought back, once, and told to read `BRIEF.md` and `PROGRESS.md` and what you left. Where your program says which conversation you are (a T3 Code thread, a herdr pane, Codex and Claude Code do), it is this same conversation that goes on, with everything you know. Otherwise a new one opens in the same folder, so write `PROGRESS.md` as if for someone else. That is how you go on after any restart, an `omarchy update` included. `hw-test fix` is only for starting on other parts, and it writes a new brief. Set it again before each restart. `hw-test resume --show` says what is left and `--cancel` takes it back. Word older than a day is dropped, so for a fault that takes days to show, a note (step 6) is where the next session starts from.

### Showing what the change did, and the pull request

1. Measure again: the test of each part you touched, and a neighbouring part or two, to show nothing else broke. Tests that take the owner are run by them.
2. `hw-test verify after <part>` prints a block with each check before and after. It can be run as often as you like. A check that works on both sides is only named. When your change moved a number and not the state (the right speaker 11 dB louder, both sides "works"), give that check a row with `--check speakers`.
3. Open the pull request **as a draft**. On omarchy-pkgs a pull request that is ready merges and publishes by itself once its checks pass, so a draft is how you keep it until the measurement is in. One fix per pull request. Its description starts with the family alone on a line, then the block:

   ```
   arm-dragon-msft

   ### Measured with hw-test

   LENOVO Yoga Slim 7 14Q8X9, before and after this change. Before: 19 Nov 2026 06:00. After: 19 Nov 2026 06:40.
   Kernel 7.3.0-72-omarchy and the same package versions on both sides. After, the computer was started from a test boot image with the changed device tree, made by `hw-test tree boot`.

   | Check | Before | After |
   | --- | --- | --- |
   | ✅ Speakers | partly: left heard at -31 dB, right not heard (measured by the test) | works: left heard at -31 dB, right heard at -32 dB (measured by the test) |

   Tested again and still working: built-in microphones, headphone jack.
   ```

4. `hw-test verify after <part> --pr <the pull request's address> --title "<the change in a line>"` records the same on the model's page, tied to the pull request. That links the fix, so there is nothing more to send.
5. Mark the pull request ready (`gh pr ready`), and tell the owner it is out under their account. On omarchy-pkgs, a first pull request from someone builds nothing until a maintainer approves the build, and its check reads "Awaiting build approval" until then. That is the normal wait, often a day or more, and nothing for you to fix.

- A check counts only when its own test ran again after the snapshot.
- What got worse is shown as plainly as what got better. Fix it or say so. Don't leave it out.
- Another owner of the same model can confirm a pull request the same way, with its address. Each computer counts once.
- `GET /api/fixes?url=<the pull request>` answers what a pull request changed over every computer that measured it. That is what a reviewer looks at.

### Cleaning up

After `verify after`: have the owner run `hw-test tree remove`, remove test packages, put back the settings you changed, and run `hw-test resume --cancel` if word is still left. Tell the owner the fix reaches them with `omarchy update` once the pull request is published. Until then the part is as broken as before, unless they ask you to leave the test entry in place.

## 6. Notes: what fits no check

A result says whether a check works. A note holds the rest: a freeze that takes a sequence of events, a way around a problem, a patch series on its way somewhere, a quirk of the model. Notes show on the model's page, other owners' agents read them, and anyone can answer one. **Read a model's notes before you start, and leave one when you stop without a fix,** saying how far you got.

Read them:

```sh
curl -s 'https://arewearmyet.com/api/notes?model=<id>'          # one model; also &check=<check> &kind=problem &status=open
curl -s 'https://arewearmyet.com/api/notes?q=dock+wake'         # every model, by words in the title or the text
curl -s https://arewearmyet.com/api/notes/<NOTE-ID>             # one note, with its answers and its logs' addresses
curl -s https://arewearmyet.com/api/notes/<NOTE-ID>/logs/kernel.log
```

Leaving `model` out lists every model's notes. Search Omarchy's issues as well before you write about a fault: `gh search issues --repo omacom/omarchy --repo omacom/omarchy-pkgs "<words>"`.

Write one. An owner at a terminal runs `hw-test note`, or `hw-test note wifi` to name the part, and is asked what happened and whether the computer was restarted since: tell them that and no more. You, without a terminal, give everything:

```sh
hw-test note --kind problem --check suspend --log previous \
  --title "Freezes on wake when a USB-C dock was plugged in during sleep" --body-file what-happens.md
hw-test note --kind upstream --check usb4 --title "USB4 host driver posted, version 3" \
  --link https://lore.kernel.org/... --body "Posted for review. Not in Omarchy's kernel yet."
hw-test note --reply <NOTE-ID> --body "Same on the HP dock. It wakes fine when the dock was in before sleep."
hw-test note --solved <NOTE-ID>          # your own problem, once it is fixed
```

- `--kind` is `problem` (something goes wrong), `workaround` (a way around a problem), `upstream` (work going on in another project) or `note` (anything else worth knowing). `--check` names the check it belongs to, and is left out when it is about no single one.
- The text says what happens, how to make it happen step by step, how often, on which kernel, and what was tried. For a dock, a screen or another device, say which. Up to 8000 characters. Longer output goes in a log.
- `--log` can be given up to four times. `kernel` is the kernel's log of this boot, `previous` that of the boot before, `boot-3` that of the third boot before this one (`journalctl --list-boots` lists them), and a file name attaches that file. After a freeze or a restart, the log to send is the one of the boot it happened in.
- Every log, file or not, is cleaned before it leaves the computer: hardware and network addresses, serial numbers, disk and install IDs, home folders, and the names of the computer, its user and its Bluetooth devices. The site cleans it once more. Tell the owner in a sentence that a log goes with the note and onto the model's page, and offer to show it. Don't print it at them unasked: `--json` prints exactly what would be sent and sends nothing. In a terminal `hw-test note` says the same in two lines and asks before it sends a log.
- `--link` works with every kind and with `--reply`. Links may point to GitHub, GitLab, lore.kernel.org, patchwork, kernel.org, freedesktop.org, Launchpad and this site.
- It prints the note's id and its address.

**A note is not a report to the Omarchy team.** When a problem needs them, open an issue too, and make the two point at each other:

1. Open the issue first. Kernel, device tree and package matters go to `omacom/omarchy-pkgs`, everything else to `omacom/omarchy`. Its text starts with the family alone on a line, as in a pull request, then the model, the kernel and the steps. Don't attach `omarchy-debug` output: it can hold names.
2. Write the note with `--link <the issue's address>`.
3. Add the note's address to the issue with `gh issue comment`.

A fault that comes and goes belongs in both places: the note with its logs, and the check itself set to `partial` with `hw-test report` (step 4).

Without `hw-test`, a note is `POST /api/notes` with `model`, `kind`, `title`, `body`, and optional `check`, `links`, `logs` (`[{"name": "kernel.log", "text": "..."}]`), `kernel`. Take the host name out of logs yourself then (`journalctl -k -b --no-hostname -o short-monotonic`). Answers go to `POST /api/notes/<NOTE-ID>/replies`, and a problem's writer closes it with `POST /api/notes/<NOTE-ID>/status` and `{"status": "solved"}`.

## API summary

The [API reference](https://arewearmyet.com/docs/api.md) has every field, limit and answer. `GET /api` lists the routes as JSON.

| Method and path | What it does |
| --- | --- |
| `GET /api/checks` | Every check: what works means, what is measured, what the owner does, and where to look when it fails (`debug`) |
| `GET /api/models` | Every computer model with its id, page and API link |
| `GET /api/models/<id>` | One model: booting, findings, owners' results, fixes, notes, wanted checks |
| `GET /api/match?chid=&compatible=&product=&sku=&family=&vendor=` | Which model a computer is, its family, and the kernel on Omarchy's channels |
| `POST /api/runs` | Send results; header `x-machine-token` |
| `POST /api/runs/<id>/results` | Add results to your run |
| `GET /api/runs/<id>` | One run as stored |
| `POST /api/fixes` | Link a fix to a model, with what it changed as measured before and after |
| `GET /api/fixes?url=` | What a pull request changed, over every computer that measured it |
| `GET /api/notes?model=&check=&kind=&status=&q=` | Notes. No `model` means every model |
| `GET /api/notes/<id>` | One note with its answers and its logs' addresses |
| `GET /api/notes/<id>/logs/<name>` | One log, as plain text |
| `POST /api/notes` | Write a note, with logs; header `x-machine-token` |
| `POST /api/notes/<id>/replies` | Answer a note |
| `POST /api/notes/<id>/status` | Mark your own problem solved or open |
| `GET /api/owners` | Every model's owners' results at once, and the computers that matched no model |

Errors come back as `{"error": "..."}` with a plain explanation. There's a limit of 30 reports a minute per address.
