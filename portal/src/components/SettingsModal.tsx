import { BaseModal } from "./BaseModal"

interface SettingsModalProps {
  open: boolean
  onClose: () => void
}

export function SettingsModal({ open, onClose }: SettingsModalProps) {
  return (
    <BaseModal open={open} title="Settings" titleId="settings-modal-title" onClose={onClose}>
      <div className="modal__body">
        {/* Content to be added in later tasks */}
      </div>
    </BaseModal>
  )
}
