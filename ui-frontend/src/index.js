import React from 'react';
import ReactDOM from 'react-dom/client';
import './index.css';
import App from './App';
import { createApiFetch } from './apiClient';

// Support being served under a sub-path (e.g. the Rancher cluster Service proxy
// at /k8s/clusters/<id>/api/v1/namespaces/<ns>/services/http:<svc>:<port>/proxy/).
// The app makes absolute "/api/..." requests; rewrite them to be relative to
// wherever index.html was loaded from so they reach the backend through the
// proxy. At the app root this is a no-op (apiBase is empty), so direct
// NodePort/Ingress access is unaffected. Static assets use relative paths via
// "homepage": "." in package.json. apiClient.js also attaches the user's token
// when the backend runs with USER_AUTH=token.
const apiBase = window.location.pathname.replace(/[^/]*$/, '').replace(/\/$/, '');
let storage = null;
try { storage = window.sessionStorage; } catch (e) { /* blocked: tokens are not cached */ }
window.fetch = createApiFetch(window.fetch.bind(window), { apiBase, storage });

const root = ReactDOM.createRoot(document.getElementById('root'));
root.render(
  <React.StrictMode>
    <App />
  </React.StrictMode>
);
