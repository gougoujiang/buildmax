- Fixed the Space switcher's label losing its association with the dropdown
  for an account in 2 or more Spaces at narrow widths: the persistent sidebar
  and the narrow navigation drawer could both render the same hardcoded id at
  once, and only one `<label>` can own it.

  > **简体中文：** [阅读中文镜像](../../zh-CN/changelog/fixed/sidebar-space-select-duplicate-id.md)
