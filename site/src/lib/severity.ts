export const SEVERITY_ORDER = ["CRITICAL", "HIGH", "MEDIUM", "LOW", "UNKNOWN"] as const;

export type Severity = (typeof SEVERITY_ORDER)[number];

export const SEVERITY_COLORS: Record<string, { bg: string; text: string; bar: string }> = {
  CRITICAL: { bg: "bg-red-950/40", text: "text-red-400", bar: "bg-red-600" },
  HIGH: { bg: "bg-orange-950/40", text: "text-orange-400", bar: "bg-orange-600" },
  MEDIUM: { bg: "bg-yellow-950/40", text: "text-yellow-400", bar: "bg-yellow-600" },
  LOW: { bg: "bg-green-950/40", text: "text-green-400", bar: "bg-green-600" },
  UNKNOWN: { bg: "bg-verity-surface", text: "text-verity-text-secondary", bar: "bg-verity-border" },
};

export function severityIndex(sev: string): number {
  const idx = SEVERITY_ORDER.indexOf(sev as Severity);
  return idx === -1 ? SEVERITY_ORDER.length : idx;
}

export function sortBySeverity<T>(items: T[], _getSeverity: (item: T) => string): T[] {
  return [...items].sort((a, b) => severityIndex(_getSeverity(a)) - severityIndex(_getSeverity(b)));
}
