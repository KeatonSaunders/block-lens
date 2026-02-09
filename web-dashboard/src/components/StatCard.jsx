export default function StatCard({ title, value, subtitle, color = 'blue' }) {
  const borderColors = {
    blue: 'border-l-blue-500',
    green: 'border-l-emerald-500',
    yellow: 'border-l-amber-500',
    purple: 'border-l-violet-500',
    red: 'border-l-red-500',
  };

  return (
    <div className={`bg-white border border-black/10 border-l-4 ${borderColors[color]} p-4`}>
      <div className="text-xs text-gray-500 uppercase tracking-wider font-medium">{title}</div>
      <div className="text-2xl font-semibold mt-1 font-mono">
        {typeof value === 'number' ? value.toLocaleString() : value}
      </div>
      {subtitle && <div className="text-sm text-gray-400 mt-1">{subtitle}</div>}
    </div>
  );
}
