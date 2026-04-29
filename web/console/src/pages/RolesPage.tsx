import { useEffect, useState } from "react";
import {
  useCreateRole,
  useDeleteRole,
  useRole,
  useRoleVersions,
  useRoles,
  useUpdateRole,
} from "../api/queries";
import type { RoleSpec } from "../api/types";
import { RoleEditor } from "../components/roles/RoleEditor";
import { RoleDiagnostics } from "../components/roles/RoleDiagnostics";
import { RoleList } from "../components/roles/RoleList";
import { useI18n } from "../i18n";

export function RolesPage() {
  const { t } = useI18n();
  const { data, isLoading } = useRoles();
  const createRole = useCreateRole();
  const updateRole = useUpdateRole();
  const deleteRole = useDeleteRole();
  const [selectedRoleID, setSelectedRoleID] = useState<string | null>(null);
  const [isCreatingNew, setIsCreatingNew] = useState(false);
  const [resetToken, setResetToken] = useState(0);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [editorRole, setEditorRole] = useState<RoleSpec | null>(null);

  const roles = data?.roles ?? [];
  const diagnostics = data?.diagnostics ?? [];
  const selectedRoleDetails = useRole(selectedRoleID);
  const selectedRoleVersions = useRoleVersions(isCreatingNew ? null : selectedRoleID);
  const hasSelectedRoleLoadError =
    !isCreatingNew &&
    selectedRoleID !== null &&
    roles.length > 0 &&
    editorRole === null &&
    selectedRoleDetails.isError;
  const isLoadingSelectedRole =
    !isCreatingNew &&
    roles.length > 0 &&
    editorRole === null &&
    !hasSelectedRoleLoadError;

  useEffect(() => {
    if (!roles.length) {
      setEditorRole(null);
      return;
    }
    if (!selectedRoleID && !isCreatingNew) {
      setEditorRole(null);
      setSelectedRoleID(roles[0].role_id);
      return;
    }
    if (selectedRoleID && !roles.some((role) => role.role_id === selectedRoleID)) {
      setEditorRole(null);
      setSelectedRoleID(roles[0].role_id);
      setIsCreatingNew(false);
    }
  }, [roles, selectedRoleID, isCreatingNew]);

  useEffect(() => {
    if (isCreatingNew || !selectedRoleID) {
      setEditorRole(null);
      return;
    }
    const detail = selectedRoleDetails.data;
    if (!detail || detail.role_id !== selectedRoleID) return;
    setEditorRole((current) => {
      if (current?.role_id === detail.role_id) return current;
      return detail;
    });
  }, [isCreatingNew, selectedRoleID, selectedRoleDetails.data]);

  async function handleSave(role: RoleSpec) {
    try {
      if (selectedRoleID && !isCreatingNew) {
        setErrorMessage(null);
        const updated = await updateRole.mutateAsync({
          roleID: selectedRoleID,
          role,
        });
        setEditorRole(updated);
        setSelectedRoleID(updated.role_id);
        setIsCreatingNew(false);
        return;
      }
      setErrorMessage(null);
      const created = await createRole.mutateAsync(role);
      setEditorRole(created);
      setSelectedRoleID(created.role_id);
      setIsCreatingNew(false);
    } catch (err) {
      setErrorMessage(t("roles.saveFailed"));
      console.error("Failed to save role:", err);
    }
  }

  async function handleDelete() {
    if (!selectedRoleID || isCreatingNew) return;
    try {
      setErrorMessage(null);
      await deleteRole.mutateAsync(selectedRoleID);
      setEditorRole(null);
      setSelectedRoleID(null);
      setIsCreatingNew(false);
    } catch (err) {
      setErrorMessage(t("roles.deleteFailed"));
      console.error("Failed to delete role:", err);
    }
  }

  function handleNewRole() {
    setErrorMessage(null);
    setEditorRole(null);
    setSelectedRoleID(null);
    setIsCreatingNew(true);
    setResetToken((value) => value + 1);
  }

  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden lg:flex-row">
      <RoleList
        roles={roles}
        selectedRoleID={selectedRoleID}
        isLoading={isLoading}
        onSelectRole={(roleID) => {
          setErrorMessage(null);
          setEditorRole(null);
          setSelectedRoleID(roleID);
          setIsCreatingNew(false);
        }}
        onNewRole={handleNewRole}
      />
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
        <RoleDiagnostics diagnostics={diagnostics} />
        {errorMessage ? (
          <div
            className="shrink-0 px-5 py-2 text-xs flex items-center gap-2"
            role="alert"
            style={{ backgroundColor: "var(--color-error-dim)", color: "var(--color-error)", borderBottom: "1px solid rgba(239,68,68,0.1)" }}
          >
            {errorMessage}
          </div>
        ) : null}
        {isLoadingSelectedRole ? (
          <div className="flex-1 flex items-center justify-center">
            <div className="animate-shimmer rounded-lg" style={{ width: 200, height: 16, backgroundColor: "var(--color-surface)" }} />
          </div>
        ) : hasSelectedRoleLoadError ? (
          <section className="flex-1 overflow-y-auto p-5" style={{ backgroundColor: "var(--color-bg)" }}>
            <div
              className="max-w-xl rounded-lg p-4 text-sm"
              style={{
                backgroundColor: "var(--color-error-dim)",
                border: "1px solid rgba(239,68,68,0.15)",
                color: "var(--color-error)",
              }}
            >
              <div role="alert">{t("roles.loadFailed", { roleID: selectedRoleID ?? "" })}</div>
              <button
                type="button"
                className="mt-3 rounded-md px-3 py-1.5 text-xs font-medium"
                onClick={() => {
                  void selectedRoleDetails.refetch();
                }}
                style={{
                  border: "1px solid var(--color-error)",
                  backgroundColor: "transparent",
                  color: "var(--color-error)",
                  cursor: "pointer",
                }}
              >
                {t("roles.retry")}
              </button>
            </div>
          </section>
        ) : (
          <RoleEditor
            role={isCreatingNew ? null : editorRole}
            versions={isCreatingNew ? [] : selectedRoleVersions.data?.versions ?? []}
            resetToken={resetToken}
            isSaving={createRole.isPending || updateRole.isPending}
            isDeleting={deleteRole.isPending}
            onSave={handleSave}
            onDelete={handleDelete}
          />
        )}
      </div>
    </div>
  );
}
