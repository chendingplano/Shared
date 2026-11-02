import { afterEach, expect, it, vi } from 'vitest';
import { page, userEvent } from '@vitest/browser/context';
import { render } from 'vitest-browser-svelte';
import EmojiSearchPicker from './EmojiSearchPicker.svelte';

afterEach(() => vi.unstubAllGlobals());

it('searches, browses a later page and selects a Unicode emoji', async () => {
	const items = (value: string, name: string) => ({unicode_id: value.codePointAt(0)!.toString(16), unicode_value:value, name_en:name, name_zh:'拉炮', description_en:name, description_zh:'拉炮', category_id:'activities',category_en:'Activities',category_zh:'活动',keywords_en:'celebration',keywords_zh:'庆祝',version:1,score:1});
	vi.stubGlobal('fetch', async (input: RequestInfo | URL) => {
		const url = String(input);
		if (url.includes('/categories')) return new Response(JSON.stringify({status:true,results:[{id:'activities',name_en:'Activities',name_zh:'活动'}]}));
		if (url.includes('/search')) return new Response(JSON.stringify({status:true,results:url.includes('offset=1') ? {items:[items('🎉','party popper')]} : {items:[items('🎈','balloon')],next_offset:1}}));
		return new Response('',{status:404});
	});
	const select = vi.fn();
	render(EmojiSearchPicker, {value:'',onSelect:select,limit:1,labels:{placeholder:'Search',empty:'None',error:'Error',loading:'Loading',all:'All',category:'Category',more:'Load more',attribution:'Credits'}});
	await expect.element(page.getByRole('button',{name:'balloon'})).toBeVisible();
	await page.getByRole('button',{name:'Load more'}).click();
	const emoji = page.getByRole('button',{name:'party popper'});
	await expect.element(emoji).toBeVisible();
	(emoji.element() as HTMLElement).focus();
	await userEvent.keyboard('{Enter}');
	expect(select).toHaveBeenCalledWith('🎉',expect.objectContaining({name_en:'party popper'}));
});

it('keeps newer results when an earlier request returns late', async () => {
	const result = (name: string, value: string) => ({unicode_id:value.codePointAt(0)!.toString(16),unicode_value:value,name_en:name,name_zh:'表情',description_en:name,description_zh:'表情',category_id:'activities',category_en:'Activities',category_zh:'活动',keywords_en:name,keywords_zh:'表情',version:1,score:1});
	vi.stubGlobal('fetch', async (input: RequestInfo | URL) => {
		const url = String(input);
		if (url.includes('/categories')) return new Response(JSON.stringify({status:true,results:[]}));
		if (!url.includes('/search')) return new Response('',{status:404});
		const q = new URL(url, 'http://localhost').searchParams.get('q');
		if (q === 'old') await new Promise((resolve) => setTimeout(resolve, 700));
		return new Response(JSON.stringify({status:true,results:{items:q === 'old' ? [result('old result','🎈')] : q === 'new' ? [result('new result','🎉')] : []}}));
	});
	render(EmojiSearchPicker, {labels:{placeholder:'Search',empty:'None',error:'Error',loading:'Loading',all:'All',category:'Category',more:'Load more',attribution:'Credits'}});
	const input = page.getByRole('searchbox');
	await input.fill('old');
	await new Promise((resolve) => setTimeout(resolve,300));
	await input.fill('new');
	await expect.element(page.getByRole('button',{name:'new result'})).toBeVisible();
	await new Promise((resolve) => setTimeout(resolve,700));
	await expect.element(page.getByRole('button',{name:'new result'})).toBeVisible();
	expect(page.getByRole('button',{name:'old result'}).elements()).toHaveLength(0);
});
