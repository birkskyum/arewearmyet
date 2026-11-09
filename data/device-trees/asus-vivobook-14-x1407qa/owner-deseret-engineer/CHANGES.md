## Device tree it boots with (Linux)

**Hardware:** ASUS Vivobook 14 X1407QA. SoC: Snapdragon X X1-26-100 (8 cores at 2.96 GHz, no boost). It is the same Purwa die as the X1P-42-100 (kernel soc_id 635), so it uses the `x1p42100` DT and firmware. GPU: Adreno X1-45. Panel: Innolux CMN N140JCA-ELK (0x1441).
**OS / kernel:** Omarchy (Arch Linux ARM), kernel 7.3.0-3.1-omarchy. The DTB is embedded in the UKI (ukify `DeviceTree=`).

**Started from:** upstream `qcom/x1p42100-asus-zenbook-a14-lcd.dtb` (ASUS Zenbook A14 UX3407QA, LCD), as shipped with this kernel. There is no upstream tree for the X1407QA. I left `model`/`compatible` unchanged (`asus,zenbook-a14-ux3407qa-lcd`) because distro detection and the ALSA UCM profile key off them.

**Attached:** `tree.dts`, produced with `dtc -I fs -O dts /sys/firmware/devicetree/base`. It matches the embedded DTB except for `/chosen`, which the bootloader fills in. Also attached: the stock and running trees decompiled with `dtc -s`, `stock-to-running.diff`, and the camera overlay source `vivobook-x1407qa-camera.dtso`.

### What I changed (stock Zenbook A14 LCD → running tree)

**Keyboard / USB** (I2C addresses checked against this laptop's own ACPI DSDT)
- `i2c5` (`/soc@0/geniqup@bc0000/i2c@b94000`):
  - Added `keyboard@3a`: `hid-over-i2c`, `hid-descr-addr = <0x01>`, wakeup-source. The Vivobook keyboard sits here, not where the Zenbook's is.
  - Added `redriver@43`: `nxp,ptn3222` eUSB2 repeater, with reset GPIO 6 and 1.8 V / 3.3 V supplies.
- `i2c8` (`/soc@0/geniqup@ac0000/i2c@a80000`), `keyboard@15`: `status = "disabled"`, because this laptop has nothing on that bus.
- `usb_mp_hsphy0` (`/soc@0/phy@88e1000`): `phys = <&ptn3222>`, so the eUSB2 PHY uses that repeater.

**Touchscreen**
- `i2c4` (`/soc@0/geniqup@bc0000/i2c@b90000`), `hdtl@17`: `status = "disabled"`. The X1407QA has no touchscreen, and the DSDT shows nothing on this bus.

**Video decode (iris)**
- `iris` (`/soc@0/video-codec@aa00000`, `qcom,x1p42100-iris`):
  - `status = "okay"`
  - `firmware-name = "qcom/x1p42100/ASUSTeK/zenbook-a14/qcvss8380_pa.mbn"`
  - The firmware comes from the Windows driver package `qcdx8380` 31.0.137.0 (vfw-3.1 rel0092). The Purwa-signed `_pa` variant is accepted by TZ on this retail unit.
  - Result: H.264/HEVC/VP9 decode is bit-exact against software, and AV1 is advertised.

**Cameras** (full source in `vivobook-x1407qa-camera.dtso`; wiring follows pir0c0pter0's Fedora work on this model)
- RGB: OmniVision OV02C10 at `cci@ac16000` bus 1, address 0x36.
  - Feeds CSIPHY4 (camss port@3) over 2 lanes, with MCLK4 at 19.2 MHz and reset on GPIO237.
  - New rails: PM8550B LDO7 at 2.8 V for AVDD/DVDD, and PM8010 LDO3 at 1.8 V for DOVDD.
- IR: Himax HM1092 at `cci@ac15000` bus 0, address 0x24.
  - Feeds CSIPHY0 (camss port@0) over 1 lane at 180 MHz, with MCLK0 at 24 MHz and reset on GPIO109.
  - New rails: PM8010 LDO7 at 2.912 V and LDO4 at 1.8 V.
  - IR emitter: PM8550 flash LED outputs 1 and 4, capped at 700 mA.
- Enabled `cci@ac15000`, `cci@ac16000`, `camss` and the PM8550 flash `led-controller@ee00`. Added `cam-rgb-default-state` / `cam-ir-default-state` pinctrl and the csiphy 0.8 V / 1.2 V supplies.
- `camss` `power-domains` changed from `<IFE_0 IFE_1 TITAN_TOP>` to `<IFE_0 TITAN_TOP TITAN_TOP>`.
  - Reason: Purwa's camcc has no IFE1 GDSC, but 7.3 camss (which uses the x1e80100 resource table) requires an "ife1" domain.
  - With the real IFE1 reference, probe times out and then hits a NULL dereference in `msm_vfe_genpd_cleanup`, which hangs boot.
  - TITAN_TOP stands in as a placeholder. Never stream through vfe1/csid1, which Purwa doesn't have.

### Kernel command line notes (not in the tree, but relevant)
- `mem_sleep_default=s2idle`: deep (S3) suspend cold-reboots on this model.
- `regulator_ignore_unused clk_ignore_unused pd_ignore_unused`: inherited from the install. Not yet re-tested without them.
- Out of tree: the Wi-Fi (WCN6855, subsystem 105b:e130) has no board data in linux-firmware. It works with the generic NFA765 BDF (the 105b:e0ce entry) repackaged under this card's board name.

### Status
| Area | Status |
| --- | --- |
| Display / GPU | Working: msm, eDP plus 2× USB-C DP and HDMI, Adreno X1-45, Vulkan via turnip |
| Keyboard, touchpad | Working |
| Wi-Fi | Working, with the BDF workaround above |
| Battery, USB-C | Working |
| Video decode (iris) | Working |
| Cameras | Both working. The IR sensor uses an out-of-tree hm1092 driver |
| Suspend | s2idle |
| Audio | Left speaker only. The right WSA884x reports a PA fault on every stream start; not yet checked in Windows |
