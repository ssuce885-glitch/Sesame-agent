import type { RoleSummary } from "../../api/types";

interface RoleListProps {
  roles: RoleSummary[];
  selectedRoleID: string | null;
  isLoading: boolean;
  onSelectRole: (roleID: string) => void;
  onNewRole: () => void;
}

export function RoleList({
  roles,
  selectedRoleID,
  isLoading,
  onSelectRole,
  onNewRole,
}: RoleListProps) {
  return (
    <section
      className="flex w-full shrink-0 flex-col lg:h-full lg:w-[280px] lg:min-w-[280px]"
      style={{
        backgroundColor: "var(--color-surface-2)",
        borderRight: "1px solid var(--color-border)",
      }}
    >
      <div
        className="px-4 py-3 flex items-center justify-between"
        style={{ borderBottom: "1px solid var(--color-border)" }}
      >
        <h2 className="text-sm font-semibold m-0" style={{ color: "var(--color-text-muted)" }}>
          Roles
        </h2>
        <button
          type="button"
          onClick={onNewRole}
          className="text-sm px-2 py-1 rounded"
          style={{
            backgroundColor: "var(--color-accent)",
            color: "#fff",
            border: "none",
            cursor: "pointer",
          }}
        >
          New role
        </button>
      </div>

      <div className="flex-1 overflow-y-auto py-2">
        {isLoading && (
          <p className="px-4 text-sm" style={{ color: "var(--color-text-muted)" }}>
            Loading...
          </p>
        )}

        {!isLoading && roles.length === 0 && (
          <p className="px-4 text-sm" style={{ color: "var(--color-text-muted)" }}>
            No roles yet.
          </p>
        )}

        {roles.map((role) => {
          const selected = role.role_id === selectedRoleID;
          return (
            <button
              key={role.role_id}
              type="button"
              aria-label={role.role_id}
              onClick={() => onSelectRole(role.role_id)}
              className="w-full text-left px-4 py-3 rounded-none"
              style={{
                backgroundColor: selected ? "var(--color-surface)" : "transparent",
                borderLeft: selected
                  ? "3px solid var(--color-accent)"
                  : "3px solid transparent",
                borderBottom: "1px solid var(--color-border)",
                cursor: "pointer",
              }}
            >
              <div
                className="text-sm font-medium truncate"
                style={{ color: selected ? "var(--color-text)" : "var(--color-text-muted)" }}
              >
                {role.role_id}
              </div>
              {role.display_name && (
                <div className="text-xs truncate mt-0.5" style={{ color: "var(--color-text-muted)" }}>
                  {role.display_name}
                </div>
              )}
            </button>
          );
        })}
      </div>
    </section>
  );
}
