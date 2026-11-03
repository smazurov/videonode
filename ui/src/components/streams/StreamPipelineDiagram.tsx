import { Fragment } from 'react';
import { Link } from 'react-router-dom';
import type { Tone } from '../../lib/stream-status';
import type { PipelineNode } from './useStreamPipeline';
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

// Sources stack on the left; with several, a bar joins their wires so they
// visibly fan into the composer. Rows are a fixed height so the bar can span
// from the first wire to the last.
function SourceGroup({ sources }: { readonly sources: readonly PipelineNode[] }) {
  return (
    <div className="relative flex shrink-0 flex-col">
      {sources.map((s) => (
        <div key={s.key} className="flex h-8 items-center gap-2.5 sm:justify-end">
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
function ChainNode({ node }: { readonly node: PipelineNode }) {
  return (
    <div className="flex h-8 shrink-0 items-center gap-2.5 sm:h-auto sm:flex-col sm:gap-1.5">
      <span className="text-sm text-fg sm:h-5 sm:leading-5"><NodeLabel node={node} /></span>
      <Dot tone={node.tone} className="order-first sm:order-none" />
      <span className={cn('text-xs sm:h-5 sm:leading-5', stateTone(node.tone))}>{node.state}</span>
    </div>
  );
}

interface StreamPipelineDiagramProps {
  readonly sources: readonly PipelineNode[];
  readonly chain: readonly PipelineNode[];
}

/** Sources → (composer) → encoder → relay → viewers, horizontal from sm. */
export function StreamPipelineDiagram({ sources, chain }: StreamPipelineDiagramProps) {
  return (
    <div className="flex flex-col py-1 sm:flex-row sm:items-center sm:overflow-x-auto">
      {sources.length > 0 && <SourceGroup sources={sources} />}
      {chain.map((node, i) => (
        <Fragment key={node.key}>
          {(i > 0 || sources.length > 0) && (
            <span className="ml-[3.5px] h-3 w-px bg-border sm:ml-0 sm:h-px sm:w-auto sm:min-w-8 sm:max-w-40 sm:flex-1" />
          )}
          <ChainNode node={node} />
        </Fragment>
      ))}
    </div>
  );
}
