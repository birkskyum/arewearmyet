# HP OmniBook 3 14 device tree

These are the files Omarchy's kernel builds for the HP OmniBook 3 14-hz0xxx, as of omarchy-pkgs
[`643e5ef`](https://github.com/omacom/omarchy-pkgs/tree/643e5efb14222497ead51ea1f909db8d40a2b6d4/pkgbuilds/linux-omarchy-aarch64)
(omacom/omarchy-pkgs#2263). They are its patches `2742` and `2743` applied in order.

- `2742` is Michael Haller's tree, as Jens Glathe's linux_ms_dev_kit carries it at `35e73a004cbe`.
- `2743` changes its compatibles, names the Wi-Fi card `pci17cb,1103` and loads the firmware from
  `qcom/x1p42100/hp/omnibook-3/`.

The board reads the firmware the laptop needs from these copies, because a patch link can only
cut out a file one patch creates, not a later patch's changes to it. The OmniBook 3 16's tree in
`../hp-omnibook-3-16-bz0/` includes `x1-hp-omnibook-3.dtsi` from here.

BSD-3-Clause, the licence line in each file.
