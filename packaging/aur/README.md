# AUR (`hfpacks-bin`)

Binary package for Arch Linux / pacman. Publish to the AUR once (requires an AUR account):

```bash
# After bumping pkgver/sha256sums in PKGBUILD:
git clone ssh://aur@aur.archlinux.org/hfpacks-bin.git
cp PKGBUILD .SRCINFO hfpacks-bin/
cd hfpacks-bin
makepkg --printsrcinfo > .SRCINFO
git add PKGBUILD .SRCINFO
git commit -m "hfpacks-bin $pkgver"
git push
```

Users:

```bash
yay -S hfpacks-bin
# or
paru -S hfpacks-bin
```
