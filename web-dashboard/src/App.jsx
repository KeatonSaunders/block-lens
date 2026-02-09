import { useState } from 'react';
import Dashboard from './components/Dashboard';
import Analytics from './components/Analytics';
import ApiDocs from './components/ApiDocs';

function App() {
  const [activeTab, setActiveTab] = useState('analytics');

  return (
    <div className="min-h-screen">
      {/* Header */}
      <header className="border-b border-black/10">
        <div className="max-w-7xl mx-auto px-6 py-5">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-3">
              <h1 className="text-xl font-semibold tracking-tight">
                <span className="font-display italic">Bitcoin</span> Observer
              </h1>
            </div>

            {/* Navigation */}
            <nav className="flex gap-1">
              {['analytics', 'observer', 'api'].map((tab) => (
                <button
                  key={tab}
                  onClick={() => setActiveTab(tab)}
                  className={`px-4 py-2 text-sm font-medium transition-colors ${
                    activeTab === tab
                      ? 'bg-black text-white'
                      : 'text-gray-600 hover:text-black hover:bg-black/5'
                  }`}
                >
                  {tab.charAt(0).toUpperCase() + tab.slice(1)}
                </button>
              ))}
            </nav>
          </div>
        </div>
      </header>

      {/* Main Content - keep components mounted to preserve state */}
      <main className="max-w-7xl mx-auto px-6 py-8">
        <div className={activeTab === 'analytics' ? '' : 'hidden'}>
          <Analytics />
        </div>
        <div className={activeTab === 'observer' ? '' : 'hidden'}>
          <Dashboard />
        </div>
        <div className={activeTab === 'api' ? '' : 'hidden'}>
          <ApiDocs />
        </div>
      </main>

      {/* Footer */}
      <footer className="border-t border-black/10 mt-12">
        <div className="max-w-7xl mx-auto px-6 py-6 text-center text-sm text-gray-500">
          Bitcoin P2P Network Observer & Graph Analytics
        </div>
      </footer>
    </div>
  );
}

export default App;
