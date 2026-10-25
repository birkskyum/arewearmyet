# Are we ARM yet?

Omarchy on ARM PCs, model by model: which ARM laptops and desktops run Omarchy today, and what each one still needs upstream.

## What it tracks

For every model in [`data/models.yaml`](data/models.yaml):

- **Omarchy**: what owners report on the current release (works or problems), and otherwise whether every piece is in place for it to work: the kernel of Omarchy's release candidate carries the model's device tree, and the image picks it by itself through systemd's hardware IDs.
- **Device tree**: the first Linux release with it, or linux-next, or a posted series.
- **Hardware IDs**: whether systemd's `src/boot/hwids/aa64` has an entry for the model's device tree.
- **Firmware**: whether every firmware file its device tree loads (its `firmware-name` lines) is in linux-firmware's newest release, merged for the next one, only partly there, or available as a known vendor download, so the laptop's own Windows isn't needed.
- **ACPI dump**: whether the aarch64-laptops project has the model's ACPI tables, which a device tree is written from.

## Sections

Each section is a group of machines that one guide can help along:

- **Snapdragon X and X2** (and the older 8cx Gen 3) start through UEFI and need a device tree, hardware IDs and firmware; owners help through the "Add your model" form, and later a tool that collects ACPI tables and firmware lists from Windows.
- **Googlebooks with Snapdragon** (`boot: android`) start like a Pixel, through Android's bootloader; owners help by reading the device tree Googlebook OS starts with ([owner guide](https://github.com/omacom/omarchy/discussions/16627)).
- **NVIDIA N1x and GB10** start through ACPI and need no device tree; owners help by reporting whether the ARM image boots.

## How the data flows

1. `pnpm collect` reads the sources and writes `data/sources.json`: device trees in Linux releases (GitHub mirror of torvalds/linux) and linux-next (sparse clone from kernel.org), hardware IDs in systemd releases and Ubuntu's stubble, firmware folders in linux-firmware (GitLab API), ACPI dumps in aarch64-laptops/build, and the kernel and systemd versions in Omarchy's package channels. A GitHub token from `GITHUB_TOKEN` or `gh auth token` is used when present.
2. `pnpm status` joins `data/models.yaml` with `data/sources.json` and writes `src/data/status.json`. It also lists everything the sources know that no model claims yet, under `unmatched`.
3. `pnpm build` renders the site from `src/data/status.json`.

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
