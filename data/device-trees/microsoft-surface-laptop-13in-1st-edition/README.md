# Microsoft Surface Laptop 13-inch device tree

This is the file Omarchy's kernel builds for the Surface Laptop 13-inch, as of omarchy-pkgs
[`1437003`](https://github.com/omacom/omarchy-pkgs/tree/143700346c4ddca238195e510cb79c876db52549/pkgbuilds/linux-omarchy-aarch64)
(omacom/omarchy-pkgs#2289, merged as 7.3-32), with every patch of that recipe applied in order.

- `x1p42100-microsoft-surface-laptop-13.dts` is `2772`: the Surface Pro 12-inch's tree with the
  laptop's touchscreen, its USB-A port and internal keyboard and touchpad, and its own firmware folder,
  `qcom/x1p42100/Microsoft/SurfaceLaptop13/`.

The board reads the firmware the laptop needs from this copy.

BSD-3-Clause, the licence line in the file.
