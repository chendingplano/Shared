<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import type { Page } from '@sveltejs/kit';

  export let url: URL; // passed from the host app

  onMount(async () => {
    const token = url.searchParams.get('token');
    if (!token) {
      alert('Invalid verification link');
      goto('/login');
      return;
    }

    try {
      const res = await fetch('http://localhost:5173/auth/email/verify', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token })
      });

      if (res.ok) {
        const data = await res.json();
        if (!data) {
            alert('Missing response data after verification (SHD_ATH_027)');
            goto('/login');
            return;
        }

        const redirect_url = data.redirect_url;
        if (redirect_url == null || redirect_url === '') {
            alert('Missing redirect URL after verification (SHD_ATH_034)');
            goto('/login');
            return;
        }
        goto(redirect_url);
        return;
      } else {
        const status = res.status;
        const statusText = res.statusText;
        const msg = "Verification failed, status:" + status + ", statusText:" + statusText + " (SHD_ATH_035)";
        console.log(msg)
        alert(msg);
        goto('/login');
        return;
      }
    } catch (err) {
      alert('Network error (SHD_ATH_047): ' + err);
      goto('/login');
    }
  });
</script>

<p>Verifying your email... (SHD_ATH_054)</p>