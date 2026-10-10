# OpenWrt feed recipe for f4 (extra-lite build): drop this directory into a
# package feed as utils/f4 and `make package/f4/compile`. Not built in the
# OpenWrt build system yet (unxed/f4#1671); the .ipk files the `openwrt`
# workflow makes with scripts/build_ipk.sh are the tested way until a feed
# maintainer has run it. Needs the SDK's Go host toolchain to be at least the
# version go.mod asks for.
include $(TOPDIR)/rules.mk

PKG_NAME:=f4
PKG_VERSION:=0.3.0-beta
PKG_RELEASE:=1

PKG_SOURCE:=$(PKG_NAME)-$(PKG_VERSION).tar.gz
PKG_SOURCE_URL:=https://codeload.github.com/unxed/f4/tar.gz/v$(PKG_VERSION)?
PKG_HASH:=skip

PKG_MAINTAINER:=unxed <https://github.com/unxed/f4>
PKG_LICENSE:=BSD-3-Clause
PKG_LICENSE_FILES:=LICENSE

PKG_BUILD_DEPENDS:=golang/host
PKG_BUILD_PARALLEL:=1
PKG_BUILD_FLAGS:=no-mips16

GO_PKG:=github.com/unxed/f4
GO_PKG_BUILD_PKG:=github.com/unxed/f4/cmd/f4
GO_PKG_TAGS:=lite,extralite,vtui_noebiten,vtui_nogogpu
GO_PKG_LDFLAGS:=-s -w

include $(INCLUDE_DIR)/package.mk
include $(TOPDIR)/feeds/packages/lang/golang/golang-package.mk

define Package/f4
  SECTION:=utils
  CATEGORY:=Utilities
  TITLE:=Efficient and cozy two-panel file manager
  URL:=https://github.com/unxed/f4
  DEPENDS:=$(GO_ARCH_DEPENDS)
endef

define Package/f4/description
  A two-panel file manager with an editor and a viewer, in the extra-lite
  profile built for routers: no GUI backends, English interface (translation
  files can be added under the language directory), no East Asian code pages.
endef

define Package/f4/install
	$(INSTALL_DIR) $(1)/usr/bin
	$(INSTALL_BIN) $(GO_PKG_BUILD_BIN_DIR)/f4 $(1)/usr/bin/f4
endef

$(eval $(call GoBinPackage,f4))
$(eval $(call BuildPackage,f4))
