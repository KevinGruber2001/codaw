import { mount } from 'svelte';
import '../lib/theme.css';
import View from '../views/MasterEditor.svelte';

mount(View, { target: document.getElementById('app')! });
