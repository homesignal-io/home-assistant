import dashboardFixture from "../../testdata/contracts/api/public-v1/dashboard.json";
import devicesFixture from "../../testdata/contracts/api/public-v1/devices.json";
import activityFixture from "../../testdata/contracts/api/public-v1/activity.json";

const apiBaseUrl = (import.meta.env.VITE_HOMESIGNAL_API_BASE_URL || "").replace(/\/$/, "");
const apiToken = import.meta.env.VITE_HOMESIGNAL_API_TOKEN || "";

async function readModel(path, fallback) {
  if (!apiBaseUrl) return fallback;

  try {
    const headers = {
      Accept: "application/json",
    };
    if (apiToken) {
      headers.Authorization = `Bearer ${apiToken}`;
    }

    const response = await fetch(`${apiBaseUrl}${path}`, {
      headers,
    });

    if (response.status === 501) return fallback;
    if (!response.ok) throw new Error(`HTTP ${response.status}`);

    return await response.json();
  } catch (error) {
    console.warn(`Falling back to contract fixture for ${path}:`, error);
    return fallback;
  }
}

export function getDashboard() {
  return readModel("/dashboard", dashboardFixture);
}

export function getDevices() {
  return readModel("/devices", devicesFixture);
}

export function getActivity() {
  return readModel("/activity", activityFixture);
}
