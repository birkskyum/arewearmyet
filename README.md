# Are we ARM yet?

Omarchy on ARM PCs, model by model: which ARM laptops and desktops run Omarchy today, and what each one still needs upstream.

## What it tracks

For every model in [`data/models.yaml`](data/models.yaml):

- **Omarchy**: whether the kernel of Omarchy's current release candidate carries the model's device tree, and whether the image picks it by itself through systemd's hardware IDs.
- **Device tree**: the first Linux release with it, or linux-next, or a posted series.
- **Hardware IDs**: whether systemd's `src/boot/hwids/aa64` has an entry for the model's device tree.
- **Firmware**: whether linux-firmware carries the model's DSP, GPU and Wi-Fi files.
- **ACPI dump**: whether the aarch64-laptops project has the model's ACPI tables, which a device tree is written from.

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
