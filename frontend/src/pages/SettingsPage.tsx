import { Routes, Route, Navigate } from 'react-router-dom';
import SettingsAPIKeysPage from './SettingsAPIKeysPage';
import SettingsProfilePage from './SettingsProfilePage';

export function SettingsPage() {
  return (
    <Routes>
      <Route path="api-keys" element={<SettingsAPIKeysPage />} />
      <Route path="profile" element={<SettingsProfilePage />} />
      <Route path="*" element={<Navigate to="profile" replace />} />
    </Routes>
  );
}

