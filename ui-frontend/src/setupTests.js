// jest-dom adds custom jest matchers for asserting on DOM nodes.
import '@testing-library/jest-dom';

// Dates are rendered with the viewer's locale and time zone. The page snapshots
// must read the same on every machine and in CI, so tests render dates as en-US
// in UTC.
const toLocale = Date.prototype.toLocaleString;
Date.prototype.toLocaleString = function (_locale, options) { // eslint-disable-line no-extend-native
  return toLocale.call(this, 'en-US', { ...options, timeZone: 'UTC' });
};
