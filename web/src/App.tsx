import { Routes, Route } from 'react-router-dom';
import Layout from './ui/Layout';
import Home from './pages/Home';
import Login from './pages/Login';
import Register from './pages/Register';
import Profile from './pages/Profile';
import Notifications from './pages/Notifications';

export default function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route path="/" element={<Home />} />
        <Route path="/login" element={<Login />} />
        <Route path="/register" element={<Register />} />
        <Route path="/u/:username" element={<Profile />} />
        <Route path="/notifications" element={<Notifications />} />
        <Route path="*" element={<p>Not found.</p>} />
      </Route>
    </Routes>
  );
}
