// Contact form -> Firestore.
//
// The page writes each submission directly to a Firestore collection using
// the Firebase Web SDK (loaded from Google's CDN). No backend server is
// involved; writes are gated by the Firestore security rules in
// ../firestore.rules (create-only, validated).
//
// SETUP (see ../README-landing.md):
//   1. Create a Firebase project and a Web App, enable Firestore.
//   2. Replace every "__FILL_ME__" below with your real web config values
//      (Firebase console -> Project settings -> Your apps -> Web app -> SDK config).
//   3. Deploy the rules and hosting:  firebase deploy
//
// Until the config is filled in, the form stays usable but reports that the
// backend isn't connected yet instead of throwing.

import { initializeApp } from "https://www.gstatic.com/firebasejs/11.6.0/firebase-app.js";
import {
  getFirestore,
  collection,
  addDoc,
  serverTimestamp,
} from "https://www.gstatic.com/firebasejs/11.6.0/firebase-firestore.js";

// ---------------------------------------------------------------------------
// Firebase config - REPLACE the placeholders with your project's values.
// ---------------------------------------------------------------------------
const firebaseConfig = {
  apiKey: "__FILL_ME__",
  authDomain: "__FILL_ME__.firebaseapp.com",
  projectId: "__FILL_ME__",
  storageBucket: "__FILL_ME__.appspot.com",
  messagingSenderId: "__FILL_ME__",
  appId: "__FILL_ME__",
};

// The Firestore collection submissions are written to. Must match the
// collection name allowed in firestore.rules.
const COLLECTION = "contactMessages";

const isConfigured = !JSON.stringify(firebaseConfig).includes("__FILL_ME__");

let db = null;
if (isConfigured) {
  try {
    db = getFirestore(initializeApp(firebaseConfig));
  } catch (err) {
    console.error("Firebase init failed:", err);
  }
}

// ---------------------------------------------------------------------------
// Form wiring
// ---------------------------------------------------------------------------
const form = document.getElementById("contactForm");
const statusEl = document.getElementById("formStatus");
const submitBtn = document.getElementById("submitBtn");

const EMAIL_RE = /^[^@\s]+@[^@\s]+\.[^@\s]+$/;

function setStatus(message, kind) {
  if (!statusEl) return;
  statusEl.textContent = message;
  statusEl.classList.remove("is-success", "is-error");
  if (kind) statusEl.classList.add(kind === "success" ? "is-success" : "is-error");
}

if (form) {
  form.addEventListener("submit", async (event) => {
    event.preventDefault();

    const email = form.email.value.trim();
    const message = form.message.value.trim();

    // Client-side validation (the Firestore rules validate again server-side).
    if (!EMAIL_RE.test(email)) {
      setStatus("Please enter a valid email address.", "error");
      form.email.focus();
      return;
    }
    if (message.length < 1) {
      setStatus("Please add a short message.", "error");
      form.message.focus();
      return;
    }
    if (message.length > 5000) {
      setStatus("That message is a bit long - please keep it under 5000 characters.", "error");
      return;
    }

    if (!isConfigured || !db) {
      setStatus(
        "The contact form isn't connected yet. Add your Firebase config in contact.js to enable it.",
        "error"
      );
      return;
    }

    submitBtn.disabled = true;
    const originalLabel = submitBtn.textContent;
    submitBtn.textContent = "Sending...";
    setStatus("");

    try {
      await addDoc(collection(db, COLLECTION), {
        email,
        message,
        createdAt: serverTimestamp(),
      });
      form.reset();
      setStatus("Thanks - your message is on its way. We'll be in touch.", "success");
    } catch (err) {
      console.error("Contact submission failed:", err);
      setStatus("Something went wrong sending your message. Please try again.", "error");
    } finally {
      submitBtn.disabled = false;
      submitBtn.textContent = originalLabel;
    }
  });
}
