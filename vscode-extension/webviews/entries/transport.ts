import { mount } from 'svelte';
import '../lib/theme.css';
import View from '../views/Transport.svelte';

mount(View, { target: document.getElementById('app')! });
