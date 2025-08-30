export default function GridBackground() {
  return (
    <div className="absolute inset-0 -z-10 overflow-hidden">
      <div className="absolute inset-0 bg-[linear-gradient(to_right,#e5e7eb30_1px,transparent_1px),linear-gradient(to_bottom,#e5e7eb30_1px,transparent_1px)] bg-[size:60px_60px] dark:bg-[linear-gradient(to_right,#37415120_1px,transparent_1px),linear-gradient(to_bottom,#37415120_1px,transparent_1px)]" />
    </div>
  );
}