import { Fragment, type CSSProperties, type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import type { Tone } from '../../lib/stream-status';
import { HOVER_HINT } from '../primitives/hoverHint';
import type { PipelineInput, PipelineNode } from './useStreamPipeline';
import { cn } from '../../utils';

const DOT_TONE: Record<Tone, string> = {
  ok: 'bg-success',
  warn: 'bg-warning',
  bad: 'bg-danger',
  off: 'bg-fg-subtle',
};

function stateTone(tone: Tone): string {
  if (tone === 'bad') return 'text-danger';
  if (tone === 'warn') return 'text-warning';
  return 'text-fg-subtle';
}

function Dot({ tone, className }: { readonly tone: Tone; readonly className?: string }) {
  return <span className={cn('inline-block h-2 w-2 shrink-0 rounded-full', DOT_TONE[tone], className)} />;
}

function NodeLabel({ node }: { readonly node: PipelineNode }) {
  return node.href ? <Link to={node.href} className="hover:underline">{node.label}</Link> : <>{node.label}</>;
}

// Grid placement only applies from sm, where the container is a grid; on
// narrow screens the same cells flow as a vertical list in DOM order.
function at(col: number, row: number): CSSProperties {
  return { gridColumn: col, gridRow: row };
}

// Sources stack on the left; with several, a bar joins their wires so they
// visibly fan into the composer. Rows are a fixed height so the bar can span
// from the first wire to the last.
function SourceGroup({ sources }: { readonly sources: readonly PipelineNode[] }) {
  return (
    <div className="relative flex shrink-0 flex-col" style={at(1, 1)}>
      {sources.map((s) => (
        <div key={s.key} className="flex h-8 items-center gap-2.5 whitespace-nowrap sm:justify-end">
          <span className="text-sm text-fg"><NodeLabel node={s} /></span>
          <span className={cn('text-xs', stateTone(s.tone))}>{s.state}</span>
          <Dot tone={s.tone} className="order-first sm:order-none" />
          <span className="hidden h-px w-5 bg-border sm:block" />
        </div>
      ))}
      {sources.length > 1 && <span className="absolute bottom-4 right-0 top-4 hidden w-px bg-border sm:block" />}
    </div>
  );
}

// Wide: name above, dot on the wire, state below, so the dot sits on the
// shared axis. Narrow: one row per hop (dot, name, state) on a vertical wire.
function ChainNode({ node, style }: { readonly node: PipelineNode; readonly style: CSSProperties }) {
  return (
    <div className="flex h-8 shrink-0 items-center gap-2.5 whitespace-nowrap sm:h-auto sm:flex-col sm:gap-1.5" style={style}>
      <span className="text-sm text-fg sm:h-5 sm:leading-5"><NodeLabel node={node} /></span>
      <Dot tone={node.tone} className="order-first sm:order-none" />
      <span className={cn('text-xs sm:h-5 sm:leading-5', stateTone(node.tone))}>{node.state}</span>
    </div>
  );
}

function Wire({ style }: { readonly style: CSSProperties }) {
  return <span className="ml-[3.5px] h-3 w-px bg-border sm:ml-0 sm:h-px sm:w-full" style={style} />;
}

/** One audio track: device, state, and the visible part of what its tooltip explains. */
function AudioInput({ input, under, style }: {
  readonly input: PipelineInput;
  readonly under: 'source' | 'node';
  readonly style: CSSProperties;
}) {
  return (
    <div
      className={cn(
        'flex h-7 items-center gap-2 whitespace-nowrap pl-5 text-xs sm:pl-0',
        under === 'source' ? 'sm:justify-end' : 'sm:w-full',
      )}
      style={style}
      title={input.title}
    >
      <span className="text-fg-subtle sm:hidden">↳ audio</span>
      <Dot tone={input.tone} />
      <span className={cn('text-fg', input.title && HOVER_HINT)}>{input.label}</span>
      <span className={stateTone(input.tone)}>{input.state}</span>
      <span className={cn('hidden h-px bg-border sm:block', under === 'source' ? 'w-5' : 'min-w-3 flex-1')} />
    </div>
  );
}

// Where an audio row's wire meets the encoder column: in from the left, then
// up to the encoder; rows above the last carry the bar on down to the next.
function Elbow({ last, style }: { readonly last: boolean; readonly style: CSSProperties }) {
  return (
    <span className="relative hidden self-stretch sm:block" style={style}>
      <span className="absolute left-0 right-1/2 top-1/2 h-px bg-border" />
      <span className={cn('absolute -top-1.5 left-1/2 w-px bg-border', last ? 'bottom-1/2' : 'bottom-0')} />
    </span>
  );
}

interface StreamPipelineDiagramProps {
  readonly sources: readonly PipelineNode[];
  readonly chain: readonly PipelineNode[];
}

/**
 * Sources → (composer) → encoder → relay → viewers, horizontal from sm. Audio
 * devices the encoder captures itself sit one per track under the encoder's
 * video input, each wired into the encoder past that node.
 */
export function StreamPipelineDiagram({ sources, chain }: StreamPipelineDiagramProps) {
  const cells: ReactNode[] = [];
  const columns: string[] = ['auto'];
  if (sources.length > 0) cells.push(<SourceGroup key="sources" sources={sources} />);

  const encoderIndex = chain.findIndex((n) => (n.inputs?.length ?? 0) > 0);
  const audio = encoderIndex >= 0 ? chain[encoderIndex]!.inputs ?? [] : [];
  // Column 1 holds the sources; chain node i sits at 3 + 2i with its incoming wire just before it.
  const nodeCol = (i: number) => 3 + 2 * i;
  const inputCol = encoderIndex > 0 ? nodeCol(encoderIndex - 1) : 1;
  const encoderCol = nodeCol(encoderIndex);

  const audioRows = audio.map((input, k) => (
    <Fragment key={input.key}>
      <AudioInput input={input} under={inputCol === 1 ? 'source' : 'node'} style={at(inputCol, 2 + k)} />
      <span className="hidden h-px w-full bg-border sm:block" style={at(encoderCol - 1, 2 + k)} />
      <Elbow last={k === audio.length - 1} style={at(encoderCol, 2 + k)} />
    </Fragment>
  ));
  if (inputCol === 1) cells.push(...audioRows);

  for (const [i, node] of chain.entries()) {
    columns.push('minmax(2rem, 1fr)', 'auto');
    if (i > 0 || sources.length > 0) cells.push(<Wire key={`wire:${node.key}`} style={at(nodeCol(i) - 1, 1)} />);
    cells.push(<ChainNode key={node.key} node={node} style={at(nodeCol(i), 1)} />);
    if (nodeCol(i) === inputCol) cells.push(...audioRows);
  }

  return (
    <div
      className="flex flex-col py-1 sm:grid sm:items-center sm:overflow-x-auto"
      style={{ gridTemplateColumns: columns.join(' ') }}
    >
      {cells}
    </div>
  );
}
