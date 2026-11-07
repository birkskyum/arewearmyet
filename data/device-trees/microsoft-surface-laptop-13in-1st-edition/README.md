# Microsoft Surface Laptop 13-inch device tree

This is the file Omarchy's kernel would build for the Surface Laptop 13-inch, as of omarchy-pkgs
[`7836eb7`](https://github.com/birkskyum/omarchy-pkgs/tree/7836eb7869134007ca12cf05ab3bdb634caa8220/pkgbuilds/linux-omarchy-aarch64)
(omacom/omarchy-pkgs#2289), with every patch of that recipe applied in order.

- `x1p42100-microsoft-surface-laptop-13.dts` is `2772`: the Surface Pro 12-inch's tree with the
  laptop's touchscreen, its USB-A port and internal keyboard and touchpad, and its own firmware folder,
  `qcom/x1p42100/Microsoft/SurfaceLaptop13/`.

The board reads the firmware the laptop needs from this copy.

BSD-3-Clause, the licence line in the file.
