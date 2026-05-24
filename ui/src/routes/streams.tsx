// Pending U12: new StreamList using DataTable + EntityDetailLayout.
// The old StreamsGrid (card grid) was deleted by U14; this stub keeps the
// build green until U12 lands.
import { useCallback } from 'react';
import { DashboardLayout } from '../components/DashboardLayout';
import { InfoBar } from '../components/InfoBar';
import { useAuthStore } from '../hooks/useAuthStore';

export default function Streams() {
  const { logout } = useAuthStore();
  const handleLogout = useCallback(() => {
    logout();
  }, [logout]);

  return (
    <DashboardLayout onLogout={handleLogout} bottomBar={<InfoBar />}>
      <DashboardLayout.MainContent>
        <div className="p-8 text-sm text-fg-muted">
          Streams list is being rebuilt (U12). Check back after the refactor.
        </div>
      </DashboardLayout.MainContent>
    </DashboardLayout>
  );
}
