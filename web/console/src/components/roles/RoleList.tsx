import type { RoleSpec } from "../../api/types";
import { useI18n } from "../../i18n";
import { Plus, Users } from "../Icon";

interface RoleListProps {
  roles: RoleSpec[];
  selectedRoleID: string | null;
  isLoading: boolean;
  onSelectRole: (roleID: string) => void;
  onCreateRole: () => void;
}

export function RoleList({
  roles,
  selectedRoleID,
  isLoading,
  onSelectRole,
  onCreateRole,
}: RoleListProps) {
  const { t } = useI18n();

  return (
    <section
      className="flex w-full shrink-0 flex-col lg:h-full lg:w-[260px] lg:min-w-[260px]"
      style={{
        backgroundColor: "var(--color-bg-elevated)",
        borderRight: "1px solid var(--color-border)",
      }}
    >
      <div
        className="px-3 py-2.5 flex items-center justify-between"
        style={{ borderBottom: "1px solid var(--color-border)" }}
      >
        <div className="flex items-center gap-2">
          <Users size={14} color="var(--color-text-tertiary)" />
          <h2 className="text-xs font-semibold uppercase tracking-wider m-0" style={{ color: "var(--color-text-secondary)" }}>
            {t("roles.title")}
          </h2>
        </div>
        <button
          type="button"
          aria-label={t("roles.create")}
          title={t("roles.create")}
          onClick={onCreateRole}
          className="inline-flex h-7 w-7 items-center justify-center rounded-md"
          style={{
            border: "1px solid var(--color-border)",
            backgroundColor: "var(--color-surface)",
            color: "var(--color-text-secondary)",
            cursor: "pointer",
          }}
        >
          <Plus size={14} color="currentColor" />
        </button>
      </div>

      <div className="flex-1 overflow-y-auto py-1">
        {isLoading && (
          <p className="px-3 text-xs" style={{ color: "var(--color-text-tertiary)" }}>
            {t("roles.loading")}
          </p>
        )}

        {!isLoading && roles.length === 0 && (
          <p className="px-3 text-xs" style={{ color: "var(--color-text-tertiary)" }}>
            {t("roles.empty")}
          </p>
        )}

        {roles.map((role) => {
          const selected = role.id === selectedRoleID;
          return (
            <button
              key={role.id}
              type="button"
              aria-label={role.id}
              onClick={() => onSelectRole(role.id)}
              className="w-full text-left px-3 py-2.5 rounded-none flex items-start gap-2"
              style={{
                backgroundColor: selected ? "rgba(59,130,246,0.06)" : "transparent",
                borderLeft: selected ? "2px solid var(--color-accent)" : "2px solid transparent",
                borderBottom: "1px solid var(--color-border)",
                cursor: "pointer",
                transition: "background-color 0.1s",
              }}
              onMouseEnter={(e) => {
                if (!selected) e.currentTarget.style.backgroundColor = "rgba(255,255,255,0.02)";
              }}
              onMouseLeave={(e) => {
                if (!selected) e.currentTarget.style.backgroundColor = "transparent";
              }}
            >
              <div className="mt-0.5 w-6 h-6 rounded-full flex items-center justify-center shrink-0" style={{ backgroundColor: "var(--color-surface)" }}>
                <span className="text-[10px] font-bold" style={{ color: "var(--color-accent)" }}>
                  {role.name.charAt(0).toUpperCase()}
                </span>
              </div>
              <div className="flex-1 min-w-0">
                <div className="flex items-center justify-between gap-2">
                  <span className="text-sm font-medium truncate" style={{ color: selected ? "var(--color-text)" : "var(--color-text-secondary)" }}>
                    {role.name}
                  </span>
                  {role.version && (
                    <span className="text-[10px] shrink-0 px-1 rounded" style={{ backgroundColor: "var(--color-surface)", color: "var(--color-text-tertiary)" }}>
                      v{role.version}
                    </span>
                  )}
                </div>
                <div className="text-xs truncate mt-0.5" style={{ color: "var(--color-text-tertiary)" }}>
                  {role.id}
                </div>
                {(role.skill_names?.length ?? 0) > 0 && (
                  <div className="flex flex-wrap gap-1 mt-1.5">
                    {role.skill_names?.slice(0, 3).map((s) => (
                      <span key={s} className="text-[10px] px-1 rounded" style={{ backgroundColor: "var(--color-surface)", color: "var(--color-text-tertiary)" }}>
                        {s}
                      </span>
                    ))}
                    {(role.skill_names?.length ?? 0) > 3 && (
                      <span className="text-[10px]" style={{ color: "var(--color-text-tertiary)" }}>
                        +{(role.skill_names?.length ?? 0) - 3}
                      </span>
                    )}
                  </div>
                )}
              </div>
            </button>
          );
        })}
      </div>
    </section>
  );
}
