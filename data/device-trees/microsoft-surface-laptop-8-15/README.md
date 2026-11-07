# Microsoft Surface Laptop 8 15-inch device tree

These are the files Omarchy's kernel builds for the Surface Laptop 8 15-inch, as of omarchy-pkgs
[`3b04e8e`](https://github.com/omacom/omarchy-pkgs/tree/3b04e8ef57e325953b30d597b0518928a7e3b6c8/pkgbuilds/linux-omarchy-aarch64)
(omacom/omarchy-pkgs#2276, merged as 7.3-30), with every patch of that recipe applied in order.

- `glymur-microsoft-surface-laptop8-15.dts` is `2764`. It includes the 13.8-inch tree and replaces the
  touchscreen and its power switch, which the 15-inch model's ACPI tables describe differently.
- `glymur-microsoft-surface-laptop8.dts` is the 13.8-inch tree as `2384` and its follow-ups leave it,
  with the DSP firmware in `qcom/glymur/microsoft/mahua/` from `2440`.

The board reads the firmware the laptop needs from these copies, because a patch link can only
cut out a file one patch creates, not a later patch's changes to it.

BSD-3-Clause, the licence line in each file.
