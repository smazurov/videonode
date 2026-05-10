import React from "react";

interface VariantMatrixProps<Row extends string, Col extends string> {
  readonly rows: readonly Row[];
  readonly cols: readonly Col[];
  readonly rowLabel?: string;
  readonly colLabel?: string;
  readonly render: (row: Row, col: Col) => React.ReactNode;
}

// Renders a 2D grid for `variant × variant` coverage (e.g. theme × size).
export function VariantMatrix<Row extends string, Col extends string>({
  rows,
  cols,
  rowLabel,
  colLabel,
  render,
}: VariantMatrixProps<Row, Col>) {
  return (
    <div className="overflow-x-auto">
      <table className="border-separate border-spacing-3">
        <thead>
          <tr>
            <th className="text-left text-xs font-mono text-fg-subtle">
              {rowLabel && colLabel ? `${rowLabel} \\ ${colLabel}` : ""}
            </th>
            {cols.map((col) => (
              <th
                key={col}
                className="text-left text-xs font-mono text-fg-muted font-normal"
              >
                {col}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={row}>
              <th className="text-left text-xs font-mono text-fg-muted font-normal pr-3">
                {row}
              </th>
              {cols.map((col) => (
                <td key={col} className="align-middle">
                  {render(row, col)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
