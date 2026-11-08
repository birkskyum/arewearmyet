# Are we ARM yet?

Linux on ARM PCs, model by model: which ARM laptops and desktops boot today, and what each one still needs upstream for every distro. Boot tests run on Omarchy, whose team keeps the board.

## What it tracks

For every model in [`data/models.yaml`](data/models.yaml):

- **Bootable**: the most stable Omarchy channel whose install image picks the model's device tree by itself, through its kernel's trees and hardware IDs. It turns green once an owner reports reaching the desktop (on Omarchy so far), and red when an owner reports problems. Without a channel, it names what the model still needs to boot.
- **Device tree**: the first Linux release with it, or linux-next, or a posted series.
- **Hardware IDs**: whether systemd's `src/boot/hwids/aa64` has an entry for the model's device tree.
- **Firmware**: whether every firmware file its device tree loads (its `firmware-name` lines) is in linux-firmware's newest release, merged for the next one, sent to linux-firmware as a merge request, only partly there, or available as a known vendor download, so the laptop's own Windows isn't needed.
- **ACPI dump**: whether the aarch64-laptops project has the model's ACPI tables, which a device tree is written from.

[`data/firmware-sources.yaml`](data/firmware-sources.yaml) lists, for the firmware linux-firmware doesn't ship yet, which maker download holds each file, where inside it, and each file's SHA-256, with no firmware in it. The site shows it at `/firmware` and serves it as `/firmware-sources.json` for tools on any distro. [`scripts/snapdragon-firmware-recipes.py`](scripts/snapdragon-firmware-recipes.py) writes Omarchy's firmware packages from it, one recipe per machine in omacom/omarchy-pkgs: `scripts/snapdragon-firmware-recipes.py --pkgbuilds <omarchy-pkgs>/pkgbuilds`.

## What it keeps

The board links its sources, and keeps its own copy of whatever no durable archive holds yet, so a closed pull request or a deleted repository loses nothing:

- [`device-trees/`](device-trees): every device tree it links that isn't in Linux, a posted series as its mbox plus the files it adds, or an owner's out-of-tree files.
- [`hwids/`](hwids): every hardware ID file that systemd's list doesn't have, from Omarchy's kernel package or Ubuntu's stubble.
- [`reports/`](reports): each hardware report from the Omarchy Arm Installer's [report repository](https://github.com/turbineBMW/omarchy-hardware-reports), merged or in an open pull request, with the ACPI tables' SoC serial number zeroed. Its device list, driver files and registry export stay there.
- [`acpi/`](acpi): the tables of every ACPI dump the board reads from [aarch64-laptops](https://github.com/aarch64-laptops/build), merged or proposed in a pull request, and from [linux-surface](https://github.com/linux-surface/acpidumps), plus dumps sent here first. [`acpi/README.md`](acpi/README.md) lists every model's dumps, here and in `reports/`. The SoC serial number is zeroed, and MSDM and SLIC, which hold the Windows product key, are left out.

Each copied folder has a `SOURCE.md` naming where its files came from. Linux, systemd and linux-firmware are linked, not copied, and firmware files are never copied, only listed in `data/firmware-sources.yaml`.

## Sections

Each section is a group of machines that one guide can help along:

- **Snapdragon X and X2** (and the older 8cx Gen 3) start through UEFI and need a device tree, hardware IDs and firmware; owners help through the "Add your model" form, with a hardware report from the [Omarchy Arm Installer](https://github.com/turbineBMW/omarchy-arm-installer) on Windows or the device tree they boot with on Linux.
- **Googlebooks with Snapdragon** (`boot: android`) start like a Pixel, through Android's bootloader; owners help by reading the device tree Googlebook OS starts with ([owner guide](https://github.com/omacom/omarchy/discussions/16627)).
- **NVIDIA N1x and GB10** start through ACPI and need no device tree; owners help by reporting whether the ARM image boots.

## How the data flows

1. `pnpm collect` reads the sources and writes `data/sources.json`: device trees in Linux releases (GitHub mirror of torvalds/linux) and linux-next (sparse clone from kernel.org), hardware IDs in systemd releases and Ubuntu's stubble, firmware folders in linux-firmware (GitLab API), ACPI dumps in aarch64-laptops/build (merged and in pull requests), linux-surface/acpidumps and the installer's hardware reports, and the kernel and systemd versions in Omarchy's package channels. A GitHub token from `GITHUB_TOKEN` or `gh auth token` is used when present.
2. `pnpm trees` and `pnpm archive` copy what the board links into `device-trees/`, `hwids/`, `reports/` and `acpi/`, as listed above.
3. `pnpm status` joins `data/models.yaml` with `data/sources.json` and writes `src/data/status.json`. It also lists everything the sources know that no model claims yet, under `unmatched`.
4. `pnpm build` renders the site from `src/data/status.json`.

## Adding a model

Add an entry to `data/models.yaml`. Quote compatible strings, since YAML splits an unquoted `vendor,model` inside `{ }` at the comma:

```yaml
- id: asus-zenbook-a14-ux3407
  vendor: ASUS
  name: Zenbook A14
  generation: x1
  released: 2025
  model_numbers: [UX3407RA, UX3407QA]
  variants:
    - { model: UX3407RA, chip: X1E-78-100, compatible: "asus,zenbook-a14-ux3407ra" }
    - { model: UX3407QA OLED, chip: X1P-42-100, compatible: "asus,zenbook-a14-ux3407qa-oled" }
  firmware: [qcom/x1e80100/ASUSTeK/zenbook-a14]
  acpi: [asus-zenbook-a14-ux3407q]
```

Then run `pnpm status` and check that the model no longer shows up under `unmatched`.

## Reporting how a model runs

Owners' reports go under the model, one per report. A report applies to every variant unless it names one:

```yaml
  reports:
    - { state: "works", note: "In daily use on Omarchy's kernel 7.2.8.", checked: "2026-10" }
    - { state: "problems", variant: "LCD", note: "The screen stays dark after install.", checked: "2026-10" }
```

The page is Astro with Tailwind; the tables are a Solid component on TanStack Table, which keeps its filters and sort in the page address.
