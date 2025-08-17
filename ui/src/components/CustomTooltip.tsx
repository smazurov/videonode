interface TooltipPayloadItem {
  payload: {
    date: number;
    [key: string]: number | string;
  };
  dataKey: string;
  value: number | string;
  color: string;
  unit?: string;
}

export interface CustomTooltipProps {
  payload: TooltipPayloadItem[];
}

export default function CustomTooltip({ payload }: Readonly<CustomTooltipProps>) {
  if (payload?.length) {
    const firstItem = payload[0];
    if (!firstItem) return null;
    const { date } = firstItem.payload;

    const getLabel = (dataKey: string) => {
      switch (dataKey) {
        case 'upload': return 'Upload';
        case 'download': return 'Download';
        case 'stat': return '';
        default: return dataKey;
      }
    };

    return (
      <div className="w-full rounded-sm border-none bg-white shadow-xs outline-1 outline-slate-800/30 dark:bg-slate-800 dark:outline-slate-300/20">
        <div className="p-2 text-black dark:text-white">
          <div className="font-semibold">
            {new Date(date * 1000).toLocaleTimeString()}
          </div>
          <div className="space-y-1">
            {payload.map((item, index) => (
              <div key={index} className="flex items-center gap-x-1">
                <div 
                  className="h-[2px] w-2" 
                  style={{ backgroundColor: item.color }}
                />
                <span>
                  {getLabel(item.dataKey)}{getLabel(item.dataKey) ? ': ' : ''}{item.value} {item.unit || "%"}
                </span>
              </div>
            ))}
          </div>
        </div>
      </div>
    );
  }

  return null;
}
