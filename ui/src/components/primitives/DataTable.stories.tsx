import { useState } from "react";
import type { Story, StoryDefault } from "@ladle/react";
import { DataTable, type DataTableColumn } from "./DataTable";
import { StatusPill, type StatusPillStatus } from "./StatusPill";

export default {
  title: "Primitives/DataTable",
} satisfies StoryDefault;

interface SourceRow {
  readonly id: string;
  readonly device: string;
  readonly status: StatusPillStatus;
  readonly consumers: number;
}

const ROWS: readonly SourceRow[] = [
  { id: "hdmi-slides", device: "/dev/video0", status: "warm", consumers: 3 },
  { id: "cam-host", device: "/dev/video2", status: "cold", consumers: 0 },
  { id: "test-pattern", device: "(test mode)", status: "running", consumers: 1 },
  { id: "usb-secondary", device: "/dev/video4", status: "error", consumers: 0 },
];

const COLUMNS: ReadonlyArray<DataTableColumn<SourceRow>> = [
  {
    id: "id",
    label: "ID",
    accessor: (r) => <code className="font-mono text-xs">{r.id}</code>,
    sort: (a, b) => a.id.localeCompare(b.id),
  },
  {
    id: "device",
    label: "Device",
    accessor: (r) => r.device,
    sort: (a, b) => a.device.localeCompare(b.device),
  },
  {
    id: "status",
    label: "Status",
    accessor: (r) => <StatusPill status={r.status} />,
    sort: (a, b) => a.status.localeCompare(b.status),
  },
  {
    id: "consumers",
    label: "Consumers",
    accessor: (r) => r.consumers,
    sort: (a, b) => a.consumers - b.consumers,
    align: "right",
  },
];

export const Basic: Story = () => (
  <DataTable<SourceRow>
    columns={COLUMNS}
    rows={ROWS}
    getRowId={(r) => r.id}
  />
);

export const WithSelection: Story = () => {
  const [selection, setSelection] = useState<readonly string[]>([]);
  return (
    <div className="space-y-3">
      <div className="text-xs text-fg-muted">Selected: {selection.join(", ") || "(none)"}</div>
      <DataTable<SourceRow>
        columns={COLUMNS}
        rows={ROWS}
        getRowId={(r) => r.id}
        selection={selection}
        onSelectionChange={setSelection}
      />
    </div>
  );
};

export const WithFilter: Story = () => {
  const [filter, setFilter] = useState("");
  return (
    <div className="space-y-3">
      <input
        type="text"
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
        placeholder="Filter…"
        className="border border-border rounded px-3 py-1.5 text-sm bg-surface text-fg"
      />
      <DataTable<SourceRow>
        columns={COLUMNS}
        rows={ROWS}
        getRowId={(r) => r.id}
        filter={filter}
      />
    </div>
  );
};

export const Empty: Story = () => (
  <DataTable<SourceRow>
    columns={COLUMNS}
    rows={[]}
    getRowId={(r) => r.id}
    emptyState="No sources configured yet."
  />
);

export const Compact: Story = () => (
  <DataTable<SourceRow>
    columns={COLUMNS}
    rows={ROWS}
    getRowId={(r) => r.id}
    density="compact"
    initialSort={{ columnId: "consumers", direction: "desc" }}
  />
);
