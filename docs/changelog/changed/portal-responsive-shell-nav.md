- Portal's narrow-width shell (roughly a phone-sized window) now shows a
  compact header with the current Space, page title, and a menu button that
  opens the full navigation in an accessible overlay drawer, instead of
  squeezing the sidebar into the page. Dialogs across Portal — including
  side-tab forms, which now use a horizontal, arrow-key-navigable tab strip at
  narrow widths — trap keyboard focus, restore it to the control that opened
  them, and become full-height sheets on narrow screens. Chat, Issues, Issue
  Detail, and Task Detail also reflow at narrow widths: the thread and
  composer no longer lose most of their width to fixed side margins, a task's
  header actions wrap under its title instead of clipping it, and primary
  action buttons meet a 44px minimum touch target. Workspace Files now shows
  either the current folder's contents or the selected file at narrow widths,
  with a Back action, instead of squeezing a fixed-width folder tree beside
  unreadably narrow content; Artifacts, admin lists (Administrators, Accounts,
  Spaces, Models, Plugins), and Space membership rows reflow to a stacked
  layout instead of wrapping into an ambiguous multi-item row; and a run's
  tool paths and token counts scroll horizontally in their own row instead of
  being cut off with an ellipsis.

  > **简体中文：** [阅读中文镜像](../../zh-CN/changelog/changed/portal-responsive-shell-nav.md)
